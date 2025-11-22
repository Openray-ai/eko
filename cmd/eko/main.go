package main

import (
	"eko/internal/api/handlers"
	"eko/internal/api/routes"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/helpers/logger"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration first
	cfg := loadConfig()

	// Initialize logger with configuration
	initializeLogger(cfg)

	logger.Info("Starting Ekō - AI Prompt Sanitization Proxy", logger.Fields{
		"version": "1.0.0",
	})

	logger.Info("Configuration loaded", logger.Fields{
		"host": cfg.Server.Host,
		"port": cfg.Server.Port,
		"log_level": cfg.Logging.Level,
		"log_format": cfg.Logging.Format,
	})

	// Initialize core components
	det := detector.New()
	san := sanitizer.New(det)

	// TODO: Load patterns from configuration
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
		"address": addr,
		"health_endpoint": "http://localhost:8080/health",
		"sanitize_endpoint": "POST http://localhost:8080/v1/sanitize",
	})

	if err := router.Run(addr); err != nil {
		logger.Fatal("Failed to start server", logger.Fields{
			"error": err.Error(),
		})
	}
}

func loadConfig() *config.Config {
	configPath := os.Getenv("EKO_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		// Use fmt here since logger isn't initialized yet
		fmt.Printf("Failed to load config from %s: %v\n", configPath, err)
		fmt.Println("Using default configuration")
		return config.Default()
	}

	return cfg
}

func initializeLogger(cfg *config.Config) {
	// Parse log level
	level, err := logger.ParseLevel(cfg.Logging.Level)
	if err != nil {
		fmt.Printf("Invalid log level '%s', using INFO: %v\n", cfg.Logging.Level, err)
		level = logger.InfoLevel
	}

	// Determine output
	output := os.Stdout
	if cfg.Logging.OutputFile != "" {
		file, err := os.OpenFile(cfg.Logging.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("Failed to open log file %s: %v\n", cfg.Logging.OutputFile, err)
			fmt.Println("Falling back to stdout")
		} else {
			output = file
		}
	}

	// Initialize logger
	logger.Initialize(logger.Config{
		Level:      level,
		Output:     output,
		JSONFormat: cfg.Logging.Format == "json",
		Colorize:   cfg.Logging.Colorize && cfg.Logging.Format != "json",
	})
}
