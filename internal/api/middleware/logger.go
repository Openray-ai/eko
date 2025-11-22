package middleware

import (
	"eko/internal/helpers/logger"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger returns a structured logging middleware
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		// Build full path with query string
		fullPath := path
		if raw != "" {
			fullPath = path + "?" + raw
		}

		// Determine log level based on status code
		fields := logger.Fields{
			"status":     statusCode,
			"method":     method,
			"path":       fullPath,
			"client_ip":  clientIP,
			"latency_ms": latency.Milliseconds(),
			"user_agent": c.Request.UserAgent(),
		}

		// Log based on status code
		if statusCode >= 500 {
			logger.Error("HTTP Request", fields)
		} else if statusCode >= 400 {
			logger.Warn("HTTP Request", fields)
		} else {
			logger.Info("HTTP Request", fields)
		}
	}
}
