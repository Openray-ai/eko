package main

import (
	"eko/internal/api/handlers"
	"eko/internal/api/routes"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"
	"eko/internal/helpers/logger"
	"eko/internal/proxy/openai"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration first
	cfg := config.LoadConfig()

	// Initialize logger with configuration
	config.InitializeLogger(cfg)

	logger.Info("Starting Ekō - AI Prompt Sanitization Proxy", logger.Fields{
		"version": "1.0.0",
	})

	logger.Info("Configuration loaded", logger.Fields{
		"host":       cfg.Server.Host,
		"port":       cfg.Server.Port,
		"log_level":  cfg.Logging.Level,
		"log_format": cfg.Logging.Format,
	})

	logger.Info("Sanitization mode configured", logger.Fields{
		"mode": cfg.Proxy.Behavior.SanitizationMode,
	})

	// Initialize core components
	det := detector.New()
	san, resolver, sessionStore := buildSanitizer(cfg, det)
	defer func() {
		if sessionStore != nil {
			_ = sessionStore.Close()
		}
	}()

	// Load default patterns
	det.LoadDefaultPatterns()

	// Initialize proxy handlers
	openaiProxy := buildOpenAIProxy(cfg, san, resolver)

	// Initialize HTTP handlers
	router := buildRouter(san, openaiProxy, sessionStore)

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	logFields := logger.Fields{
		"address":            addr,
		"health_endpoint":    "http://localhost:8080/health",
		"readiness_endpoint": "http://localhost:8080/ready",
		"metrics_endpoint":   "GET http://localhost:8080/metrics",
		"sanitize_endpoint":  "POST http://localhost:8080/v1/sanitize",
	}

	if cfg.Proxy.OpenAI.Enabled {
		logFields["openai_chat_completions"] = "POST http://localhost:8080/v1/chat/completions"
		logFields["openai_responses"] = "POST http://localhost:8080/v1/responses"
	}

	logger.Info("Ekō server starting", logFields)

	if err := router.Run(addr); err != nil {
		logger.Fatal("Failed to start server", logger.Fields{
			"error": err.Error(),
		})
	}
}

func buildSanitizer(cfg *config.Config, det *detector.Detector) (*sanitizer.Sanitizer, *tokenizer.Resolver, tokenizer.SessionStore) {
	mode := cfg.Proxy.Behavior.SanitizationMode
	if mode == "tokenize" {
		store, err := buildSessionStore(cfg)
		if err != nil {
			logger.Fatal("Failed to initialize session store", logger.Fields{
				"error": err.Error(),
			})
		}
		tok := tokenizer.NewTokenizer()
		resolver := tokenizer.NewResolver(store)
		return sanitizer.NewWithTokenizer(det, tok, store, mode), resolver, store
	}

	return sanitizer.New(det), nil, nil
}

func buildOpenAIProxy(cfg *config.Config, san *sanitizer.Sanitizer, resolver *tokenizer.Resolver) *openai.Proxy {
	if !cfg.Proxy.OpenAI.Enabled {
		return nil
	}

	openaiProxy := openai.NewWithResolver(san, resolver, cfg.Proxy.OpenAI.BaseURL, cfg.Proxy.OpenAI.Timeout)
	logger.Info("OpenAI proxy initialized", logger.Fields{
		"base_url": cfg.Proxy.OpenAI.BaseURL,
		"timeout":  cfg.Proxy.OpenAI.Timeout,
	})
	return openaiProxy
}

func buildRouter(san *sanitizer.Sanitizer, openaiProxy *openai.Proxy, sessionStore tokenizer.SessionStore) *gin.Engine {
	metrics := handlers.NewMetricsCollector()
	sanitizeHandler := handlers.NewSanitizeHandler(san)
	var healthChecker tokenizer.HealthChecker
	if checker, ok := sessionStore.(tokenizer.HealthChecker); ok {
		healthChecker = checker
	}
	healthHandler := handlers.NewHealthHandler(healthChecker)
	metricsHandler := handlers.NewMetricsHandler(metrics)

	router := gin.New()
	routes.SetupRoutes(router, sanitizeHandler, healthHandler, metricsHandler, openaiProxy)
	return router
}

func buildSessionStore(cfg *config.Config) (tokenizer.SessionStore, error) {
	ttl := time.Duration(cfg.Proxy.Behavior.TokenTTLms) * time.Millisecond
	if cfg.Proxy.Behavior.TokenStoreBackend == "redis" {
		keyProvider, err := buildKeyProvider(cfg)
		if err != nil {
			return nil, err
		}
		return tokenizer.NewRedisStore(tokenizer.RedisStoreConfig{
			Addr:          cfg.Proxy.Redis.Addr,
			Username:      cfg.Proxy.Redis.Username,
			Password:      cfg.Proxy.Redis.Password,
			DB:            cfg.Proxy.Redis.DB,
			KeyPrefix:     cfg.Proxy.Redis.KeyPrefix,
			MetaSuffix:    cfg.Proxy.Redis.MetaSuffix,
			PayloadSuffix: cfg.Proxy.Redis.PayloadSuffix,
			PoolSize:      cfg.Proxy.Redis.PoolSize,
			MinIdleConns:  cfg.Proxy.Redis.MinIdleConns,
			DialTimeout:   time.Duration(cfg.Proxy.Redis.DialTimeoutMs) * time.Millisecond,
			ReadTimeout:   time.Duration(cfg.Proxy.Redis.ReadTimeoutMs) * time.Millisecond,
			WriteTimeout:  time.Duration(cfg.Proxy.Redis.WriteTimeoutMs) * time.Millisecond,
			MaxRetries:    cfg.Proxy.Redis.MaxRetries,
			TTL:           ttl,
			MaxTokens:     cfg.Proxy.Behavior.MaxTokensPerVault,
			HealthTimeout: 2 * time.Second,
			KeyProvider:   keyProvider,
		})
	}

	return tokenizer.NewVaultManager(
		ttl,
		tokenizer.WithMaxVaults(cfg.Proxy.Behavior.MaxVaults),
		tokenizer.WithMaxTokensPerVault(cfg.Proxy.Behavior.MaxTokensPerVault),
	), nil
}

func buildKeyProvider(cfg *config.Config) (tokenizer.KeyProvider, error) {
	switch cfg.Proxy.Crypto.Provider {
	case "local":
		fallback := make(map[string]string, len(cfg.Proxy.Crypto.FallbackKeys))
		for _, key := range cfg.Proxy.Crypto.FallbackKeys {
			fallback[key.KeyID] = key.MasterKey
		}
		return tokenizer.NewStaticKeyProviderWithFallbackBase64(cfg.Proxy.Crypto.ActiveKeyID, cfg.Proxy.Crypto.LocalMasterKey, fallback)
	case "vault-transit":
		return tokenizer.NewVaultTransitKeyProvider(tokenizer.VaultTransitKeyProviderConfig{
			Address:       cfg.Proxy.Crypto.VaultTransit.Address,
			Token:         cfg.Proxy.Crypto.VaultTransit.Token,
			Namespace:     cfg.Proxy.Crypto.VaultTransit.Namespace,
			Mount:         cfg.Proxy.Crypto.VaultTransit.Mount,
			KeyName:       cfg.Proxy.Crypto.VaultTransit.KeyName,
			ActiveKeyID:   cfg.Proxy.Crypto.ActiveKeyID,
			Timeout:       time.Duration(cfg.Proxy.Crypto.VaultTransit.TimeoutMs) * time.Millisecond,
			TLSSkipVerify: cfg.Proxy.Crypto.VaultTransit.TLSSkipVerify,
		})
	default:
		return nil, fmt.Errorf("unsupported crypto provider %q", cfg.Proxy.Crypto.Provider)
	}
}
