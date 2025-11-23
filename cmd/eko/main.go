package main

import (
	"eko/internal/api/handlers"
	"eko/internal/api/routes"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/helpers/logger"
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

	// TODO: Initialize proxy handlers

	// Initialize HTTP handlers
	sanitizeHandler := handlers.NewSanitizeHandler(san)
	healthHandler := handlers.NewHealthHandler()

	// Setup router
	router := gin.New()
	routes.SetupRoutes(router, sanitizeHandler, healthHandler)

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	logger.Info("Ekō server starting", logger.Fields{
		"address":           addr,
		"health_endpoint":   "http://localhost:8080/health",
		"sanitize_endpoint": "POST http://localhost:8080/v1/sanitize",
	})

	if err := router.Run(addr); err != nil {
		logger.Fatal("Failed to start server", logger.Fields{
			"error": err.Error(),
		})
	}
}
