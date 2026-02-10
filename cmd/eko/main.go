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
	san, resolver := buildSanitizer(cfg, det)

	// Load default patterns
	det.LoadDefaultPatterns()

	// Initialize proxy handlers
	openaiProxy := buildOpenAIProxy(cfg, san, resolver)

	// Initialize HTTP handlers
	router := buildRouter(san, openaiProxy)

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	logFields := logger.Fields{
		"address":           addr,
		"health_endpoint":   "http://localhost:8080/health",
		"sanitize_endpoint": "POST http://localhost:8080/v1/sanitize",
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

func buildSanitizer(cfg *config.Config, det *detector.Detector) (*sanitizer.Sanitizer, *tokenizer.Resolver) {
	mode := cfg.Proxy.Behavior.SanitizationMode
	if mode == "tokenize" {
		vaultManager := tokenizer.NewVaultManager(
			time.Duration(cfg.Proxy.Behavior.TokenTTLms)*time.Millisecond,
			tokenizer.WithMaxVaults(cfg.Proxy.Behavior.MaxVaults),
			tokenizer.WithMaxTokensPerVault(cfg.Proxy.Behavior.MaxTokensPerVault),
		)
		tok := tokenizer.NewTokenizer()
		resolver := tokenizer.NewResolver(vaultManager)
		return sanitizer.NewWithTokenizer(det, tok, vaultManager, mode), resolver
	}

	return sanitizer.New(det), nil
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

func buildRouter(san *sanitizer.Sanitizer, openaiProxy *openai.Proxy) *gin.Engine {
	sanitizeHandler := handlers.NewSanitizeHandler(san)
	healthHandler := handlers.NewHealthHandler()

	router := gin.New()
	routes.SetupRoutes(router, sanitizeHandler, healthHandler, openaiProxy)
	return router
}
