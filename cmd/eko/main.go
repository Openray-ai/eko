package main

import (
	"eko/internal/api/handlers"
	"eko/internal/api/routes"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/core/slm"
	"eko/internal/core/tokenizer"
	"eko/internal/helpers/logger"
	"eko/internal/proxy/anthropic"
	"eko/internal/proxy/common"
	"eko/internal/proxy/deepseek"
	"eko/internal/proxy/gemini"
	"eko/internal/proxy/openai"
	proxyrouter "eko/internal/proxy/router"
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

	// Metrics collector is shared between the SLM client and the /metrics handler.
	metrics := handlers.NewMetricsCollector()

	// Initialize core components
	det := detector.New()
	if cfg.Proxy.SLM.Enabled {
		attachSLM(det, &cfg.Proxy.SLM, metrics)
	}
	san, resolver, sessionStore := buildSanitizer(cfg, det)
	defer func() {
		if sessionStore != nil {
			_ = sessionStore.Close()
		}
	}()

	// Load default patterns
	det.LoadDefaultPatterns()

	// Initialize proxy handlers
	proxyRouter := buildProxyRouter(cfg, san, resolver)

	// Initialize HTTP handlers
	router := buildRouter(cfg, metrics, san, proxyRouter, sessionStore)

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	logFields := logger.Fields{
		"address":            addr,
		"health_endpoint":    "http://localhost:8080/health",
		"readiness_endpoint": "http://localhost:8080/ready",
		"metrics_endpoint":   "GET http://localhost:8080/metrics",
		"sanitize_endpoint":  "POST http://localhost:8080/v1/sanitize",
		"batch_endpoint":     "POST http://localhost:8080/v1/sanitize/batch",
	}

	if proxyRouter != nil {
		logFields["chat_completions"] = "POST http://localhost:8080/v1/chat/completions"
		logFields["responses"] = "POST http://localhost:8080/v1/responses"
	}

	logger.Info("Ekō server starting", logFields)

	if err := router.Run(addr); err != nil {
		logger.Fatal("Failed to start server", logger.Fields{
			"error": err.Error(),
		})
	}
}

func buildSanitizer(cfg *config.Config, det *detector.Detector) (*sanitizer.Sanitizer, *tokenizer.Resolver, tokenizer.SessionStore) {
	// The tokenizer plumbing is wired up unconditionally so the per-request
	// `sanitization_mode` override on POST /v1/sanitize works regardless of
	// the configured default. The configured value becomes the default mode
	// applied when a request doesn't set its own.
	mode := cfg.Proxy.Behavior.SanitizationMode
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

func buildProxyRouter(cfg *config.Config, san *sanitizer.Sanitizer, resolver *tokenizer.Resolver) *proxyrouter.Proxy {
	if !cfg.Proxy.OpenAI.Enabled && !cfg.Proxy.Anthropic.Enabled && !cfg.Proxy.Gemini.Enabled && !cfg.Proxy.DeepSeek.Enabled {
		return nil
	}

	adapters := []common.Adapter{}
	if cfg.Proxy.OpenAI.Enabled {
		adapters = append(adapters, openai.NewAdapter(cfg.Proxy.OpenAI.BaseURL, cfg.Proxy.OpenAI.Timeout))
	}
	if cfg.Proxy.Anthropic.Enabled {
		adapters = append(adapters, anthropic.New(cfg.Proxy.Anthropic.BaseURL, cfg.Proxy.Anthropic.Timeout))
	}
	if cfg.Proxy.Gemini.Enabled {
		adapters = append(adapters, gemini.New(cfg.Proxy.Gemini.BaseURL, cfg.Proxy.Gemini.Timeout))
	}
	if cfg.Proxy.DeepSeek.Enabled {
		adapters = append(adapters, deepseek.New(cfg.Proxy.DeepSeek.BaseURL, cfg.Proxy.DeepSeek.Timeout))
	}

	proxy := proxyrouter.New(san, resolver, proxyrouter.RoutingConfig{
		DefaultProvider: providerName(cfg.Proxy.ModelRouting.DefaultProvider),
		Models:          providerNameMap(cfg.Proxy.ModelRouting.Models),
		Prefixes:        providerNameMap(cfg.Proxy.ModelRouting.Prefixes),
	}, adapters)
	logger.Info("Model routed proxy initialized", logger.Fields{
		"default_provider":  cfg.Proxy.ModelRouting.DefaultProvider,
		"openai_enabled":    cfg.Proxy.OpenAI.Enabled,
		"anthropic_enabled": cfg.Proxy.Anthropic.Enabled,
		"gemini_enabled":    cfg.Proxy.Gemini.Enabled,
		"deepseek_enabled":  cfg.Proxy.DeepSeek.Enabled,
	})
	return proxy
}

func providerName(value string) common.ProviderName {
	if value == "google" {
		return common.ProviderGemini
	}
	return common.ProviderName(value)
}

func providerNameMap(values map[string]string) map[string]common.ProviderName {
	out := make(map[string]common.ProviderName, len(values))
	for key, value := range values {
		out[key] = providerName(value)
	}
	return out
}

func buildRouter(cfg *config.Config, metrics *handlers.MetricsCollector, san *sanitizer.Sanitizer, proxyRouter *proxyrouter.Proxy, sessionStore tokenizer.SessionStore) *gin.Engine {
	sanitizeHandler := handlers.NewSanitizeHandler(san)
	batchSanitizeHandler := handlers.NewBatchSanitizeHandler(san, handlers.BatchSanitizeLimits{
		MaxBatchItems:       cfg.Proxy.Behavior.MaxBatchItems,
		MaxPromptBytes:      cfg.Proxy.Behavior.MaxPromptBytes,
		MaxBatchBytes:       int64(cfg.Proxy.Behavior.MaxBatchBytes),
		MaxBatchConcurrency: cfg.Proxy.Behavior.MaxBatchConcurrency,
	}, metrics)
	var healthChecker tokenizer.HealthChecker
	if checker, ok := sessionStore.(tokenizer.HealthChecker); ok {
		healthChecker = checker
	}
	healthHandler := handlers.NewHealthHandler(healthChecker)
	metricsHandler := handlers.NewMetricsHandler(metrics)

	router := gin.New()
	routes.SetupRoutes(router, sanitizeHandler, batchSanitizeHandler, healthHandler, metricsHandler, proxyRouter)
	return router
}

func attachSLM(det *detector.Detector, cfg *config.SLMConfig, metrics *handlers.MetricsCollector) {
	overrides := make(map[string]slm.LabelMapping, len(cfg.Labels))
	for label, ov := range cfg.Labels {
		overrides[label] = slm.LabelMapping{
			Type:     ov.Type,
			Severity: ov.Severity,
			Pattern:  ov.Pattern,
		}
	}
	client := slm.NewClient(slm.Config{
		Endpoint:      cfg.Endpoint,
		Timeout:       time.Duration(cfg.TimeoutMs) * time.Millisecond,
		MaxInputBytes: cfg.MaxInputBytes,
		Labels:        slm.MergeLabelOverrides(slm.DefaultLabels(), overrides),
		Breaker: slm.BreakerConfig{
			FailureThreshold: cfg.Breaker.FailureThreshold,
			Cooldown:         time.Duration(cfg.Breaker.CooldownMs) * time.Millisecond,
		},
		Metrics: metrics,
	})
	det.SetSLM(client)
	logger.Info("SLM contextual detector initialized", logger.Fields{
		"endpoint":   cfg.Endpoint,
		"timeout_ms": cfg.TimeoutMs,
	})
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
