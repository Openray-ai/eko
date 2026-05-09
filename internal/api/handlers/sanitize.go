package handlers

import (
	"eko/internal/core/sanitizer"
	"eko/internal/core/slm"
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
	// SLM is a per-request opt-in for the contextual SLM detector. Distinct
	// from the process-level `proxy.slm.enabled` config flag (which controls
	// whether the SLM client is wired up at startup): when SLM is wired up,
	// this flag toggles whether *this* request uses it. Defaults to false
	// when omitted — callers must opt in explicitly. Soft-fail behavior is
	// preserved: if the sidecar is unreachable or the breaker is open, the
	// request still succeeds with regex-only results.
	SLM bool `json:"slm"`
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

	ctx := slm.WithRequestEnabled(c.Request.Context(), req.SLM)
	result, err := h.sanitizer.SanitizeWithContext(ctx, req.Prompt, sessionID)
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
