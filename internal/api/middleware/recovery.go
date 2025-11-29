package middleware

import (
	"eko/internal/helpers/logger"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// Recovery returns a panic recovery middleware with structured logging
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get stack trace
				stack := string(debug.Stack())

				// Log the panic with full context
				logger.Error("Panic recovered", logger.Fields{
					"error":       fmt.Sprintf("%v", err),
					"path":        c.Request.URL.Path,
					"method":      c.Request.Method,
					"client_ip":   c.ClientIP(),
					"stack_trace": stack,
				})

				// Return error response
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
