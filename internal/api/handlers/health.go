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

// HandleLiveness reports whether the process itself is alive. It deliberately
// does not probe external dependencies — that belongs to /ready — so a flaky
// backing service cannot trigger pod restarts under K8s liveness probes.
func (h *HealthHandler) HandleLiveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "eko",
	})
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
