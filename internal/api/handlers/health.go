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

func (h *HealthHandler) HandleLiveness(c *gin.Context) {
	response := gin.H{
		"status":  "healthy",
		"service": "eko",
	}

	if h.checker != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := h.checker.HealthCheck(ctx); err != nil {
			response["dependency_status"] = "degraded"
			response["dependency_error"] = err.Error()
			c.JSON(http.StatusOK, response)
			return
		}
		response["dependency_status"] = "healthy"
	}

	c.JSON(http.StatusOK, response)
}

func (h *HealthHandler) HandleReadiness(c *gin.Context) {
	if h.checker == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ready",
			"service": "eko",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.checker.HealthCheck(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "not_ready",
			"service": "eko",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ready",
		"service": "eko",
	})
}
