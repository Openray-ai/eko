package handlers

import (
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SanitizeHandler struct {
	sanitizer *sanitizer.Sanitizer
}

func NewSanitizeHandler(s *sanitizer.Sanitizer) *SanitizeHandler {
	return &SanitizeHandler{
		sanitizer: s,
	}
}

type SanitizeRequest struct {
	Prompt    string `json:"prompt" binding:"required"`
	SessionID string `json:"session_id"`
}

func (h *SanitizeHandler) Handle(c *gin.Context) {
	var req SanitizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = tokenizer.GenerateSessionID()
	} else if err := tokenizer.ValidateSessionID(sessionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	result, err := h.sanitizer.SanitizeWithContext(c.Request.Context(), req.Prompt, sessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, tokenizer.ErrSessionStoreUnavailable) || errors.Is(err, tokenizer.ErrSessionStoreConflict) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": "sanitization failed"})
		return
	}

	c.JSON(http.StatusOK, result)
}
