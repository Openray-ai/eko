package middleware

import (
	"eko/internal/config"
	"eko/internal/helpers/logger"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	cfg, err := config.Load("configs/cors.yml")
	allowedOrigins := []string{}

	if err != nil {
		logger.Warn("Failed to load CORS config, defaulting to allow all origins", logger.Fields{
			"error":       err.Error(),
			"config_path": "configs/cors.yml",
		})
	} else {
		allowedOrigins = cfg.Cors.AllowedOrigins
	}

	defaultConfig := cors.DefaultConfig()
	defaultConfig.AllowOrigins = allowedOrigins
	defaultConfig.AllowAllOrigins = len(allowedOrigins) == 0
	defaultConfig.AllowCredentials = true
	defaultConfig.AllowHeaders = []string{
		"Content-Type", "Content-Length", "Accept-Encoding",
		"X-CSRF-Token", "Authorization", "accept", "origin",
		"Cache-Control", "X-Requested-With",
	}
	defaultConfig.ExposeHeaders = []string{"Content-Length", "Content-Type", "Content-Disposition"}

	if defaultConfig.AllowAllOrigins {
		logger.Warn("CORS configured to allow all origins - this may be a security risk in production", logger.Fields{
			"allow_all_origins": true,
		})
	}

	return cors.New(defaultConfig)
}
