package routes

import (
	"eko/internal/api/handlers"
	"eko/internal/api/middleware"
	"eko/internal/proxy/openai"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(
	router *gin.Engine,
	sanitizeHandler *handlers.SanitizeHandler,
	healthHandler *handlers.HealthHandler,
	openaiProxy *openai.Proxy,
) {
	// Apply global middleware
	router.Use(middleware.Logger())
	router.Use(middleware.Recovery())
	router.Use(middleware.CORS())

	// Health and readiness endpoints
	router.GET("/health", healthHandler.HandleLiveness)
	router.GET("/ready", healthHandler.HandleReadiness)

	// Core API v1
	v1 := router.Group("/v1")
	{
		// Sanitization endpoint
		v1.POST("/sanitize", sanitizeHandler.Handle)

		// OpenAI-compatible proxy endpoints
		if openaiProxy != nil {
			v1.POST("/chat/completions", openaiProxy.HandleChatCompletion)
			v1.POST("/responses", openaiProxy.HandleResponse)
		}

		// TODO: Add metrics endpoint
		// TODO: Add patterns management endpoints
	}

	// TODO: /v1/anthropic/* routes
	// TODO: /v1/google/* routes
}
