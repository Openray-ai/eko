package main

import (
	"eko/internal/api/handlers"
	"eko/internal/api/routes"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("Starting Ekō - AI Prompt Sanitization Proxy")

	// Load configuration
	cfg := loadConfig()
	log.Printf("Loaded configuration: server listening on %s:%s", cfg.Server.Host, cfg.Server.Port)

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
	log.Printf("Ekō is running on %s", addr)
	log.Println("Health check: http://localhost:8080/health")
	log.Println("Sanitize API: POST http://localhost:8080/v1/sanitize")

	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func loadConfig() *config.Config {
	configPath := os.Getenv("EKO_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Printf("Failed to load config from %s: %v", configPath, err)
		log.Println("Using default configuration")
		return config.Default()
	}

	return cfg
}
