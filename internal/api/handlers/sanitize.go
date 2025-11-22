package handlers

import (
	"eko/internal/core/sanitizer"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SanitizeHandler handles the core sanitization API
type SanitizeHandler struct {
	sanitizer *sanitizer.Sanitizer
}

// NewSanitizeHandler creates a new sanitize handler
func NewSanitizeHandler(s *sanitizer.Sanitizer) *SanitizeHandler {
	return &SanitizeHandler{
		sanitizer: s,
	}
}

// SanitizeRequest represents the request body for sanitization
type SanitizeRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

// Handle processes a sanitization request
func (h *SanitizeHandler) Handle(c *gin.Context) {
	var req SanitizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.sanitizer.Sanitize(req.Prompt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "sanitization failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}
