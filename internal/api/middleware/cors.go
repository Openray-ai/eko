package middleware

import (
	"eko/internal/config"
	"eko/internal/helpers/logger"
	"os"
	"strings"

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

	// EKO_ALLOWED_ORIGINS overrides the YAML so PaaS deploys (Fly/Railway/Cloud
	// Run) can set the frontend origin per environment without rebuilding.
	if v := os.Getenv("EKO_ALLOWED_ORIGINS"); v != "" {
		envOrigins := make([]string, 0)
		for o := range strings.SplitSeq(v, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				envOrigins = append(envOrigins, trimmed)
			}
		}
		allowedOrigins = envOrigins
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
