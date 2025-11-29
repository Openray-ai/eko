package main

import (
	"eko/internal/api/handlers"
	"eko/internal/api/routes"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/helpers/logger"
	"eko/internal/proxy/openai"
	"fmt"

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

	// Initialize core components
	det := detector.New()
	san := sanitizer.New(det)

	// Load default patterns
	det.LoadDefaultPatterns()

	// Initialize proxy handlers
	var openaiProxy *openai.Proxy
	if cfg.Proxy.OpenAI.Enabled {
		openaiProxy = openai.New(san, cfg.Proxy.OpenAI.BaseURL, cfg.Proxy.OpenAI.Timeout)
		logger.Info("OpenAI proxy initialized", logger.Fields{
			"base_url": cfg.Proxy.OpenAI.BaseURL,
			"timeout":  cfg.Proxy.OpenAI.Timeout,
		})
	}

	// Initialize HTTP handlers
	sanitizeHandler := handlers.NewSanitizeHandler(san)
	healthHandler := handlers.NewHealthHandler()

	// Setup router
	router := gin.New()
	routes.SetupRoutes(router, sanitizeHandler, healthHandler, openaiProxy)

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
