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
	batchSanitizeHandler *handlers.BatchSanitizeHandler,
	healthHandler *handlers.HealthHandler,
	metricsHandler *handlers.MetricsHandler,
	openaiProxy *openai.Proxy,
) {
	// Apply global middleware
	router.Use(middleware.Logger())
	router.Use(middleware.Recovery())
	router.Use(middleware.CORS())

	// Health and readiness endpoints
	router.GET("/health", healthHandler.HandleLiveness)
	router.GET("/ready", healthHandler.HandleReadiness)

	// Prometheus-compatible metrics endpoint
	router.GET("/metrics", metricsHandler.Handle)

	// Core API v1
	v1 := router.Group("/v1")
	{
		// Sanitization endpoint
		v1.POST("/sanitize", sanitizeHandler.Handle)
		v1.POST("/sanitize/batch", batchSanitizeHandler.Handle)

		// OpenAI-compatible proxy endpoints
		if openaiProxy != nil {
			v1.POST("/chat/completions", openaiProxy.HandleChatCompletion)
			v1.POST("/responses", openaiProxy.HandleResponse)
		}
	}
}
