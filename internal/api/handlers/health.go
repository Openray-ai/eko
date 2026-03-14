package handlers

import (
	"context"
	"net/http"
	"time"

	"eko/internal/core/tokenizer"
	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	checker tokenizer.HealthChecker
}

func NewHealthHandler(checker tokenizer.HealthChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

func (h *HealthHandler) Handle(c *gin.Context) {
	if h.checker != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := h.checker.HealthCheck(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "unhealthy",
				"service": "eko",
				"error":   err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "eko",
	})
}
