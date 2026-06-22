package routes

import (
	"eko/internal/api/handlers"
	"eko/internal/api/middleware"
	proxyrouter "eko/internal/proxy/router"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(
	router *gin.Engine,
	sanitizeHandler *handlers.SanitizeHandler,
	healthHandler *handlers.HealthHandler,
	metricsHandler *handlers.MetricsHandler,
	proxyRouter *proxyrouter.Proxy,
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

		// OpenAI-compatible multi-provider proxy endpoints
		if proxyRouter != nil {
			v1.POST("/chat/completions", proxyRouter.HandleChatCompletion)
			v1.POST("/responses", proxyRouter.HandleResponse)
		}
	}
}
