package routes

import (
	"eko/internal/api/handlers"
	"eko/internal/api/middleware"

	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all API routes
func SetupRoutes(
	router *gin.Engine,
	sanitizeHandler *handlers.SanitizeHandler,
	healthHandler *handlers.HealthHandler,
) {
	// Apply global middleware
	router.Use(middleware.Logger())
	router.Use(middleware.Recovery())
	router.Use(middleware.CORS())

	// Health check endpoint
	router.GET("/health", healthHandler.Handle)

	// Core API v1
	v1 := router.Group("/v1")
	{
		// Sanitization endpoint
		v1.POST("/sanitize", sanitizeHandler.Handle)

		// TODO: Add metrics endpoint
		// TODO: Add patterns management endpoints
	}

	// Proxy endpoints will be added here
	// TODO: /v1/openai/* routes
	// TODO: /v1/anthropic/* routes
	// TODO: /v1/google/* routes
}
