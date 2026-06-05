package handlers

import (
	"eko/internal/core/sanitizer"
	"eko/internal/core/slm"
	"eko/internal/core/tokenizer"
	"eko/internal/helpers/logger"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type BatchSanitizeLimits struct {
	MaxBatchItems       int
	MaxPromptBytes      int
	MaxBatchBytes       int64
	MaxBatchConcurrency int
}

type BatchSanitizeHandler struct {
	sanitizer *sanitizer.Sanitizer
	limits    BatchSanitizeLimits
	metrics   *MetricsCollector
}

func NewBatchSanitizeHandler(s *sanitizer.Sanitizer, limits BatchSanitizeLimits, metrics *MetricsCollector) *BatchSanitizeHandler {
	return &BatchSanitizeHandler{
		sanitizer: s,
		limits:    normalizeBatchLimits(limits),
		metrics:   metrics,
	}
}

type BatchSanitizeRequest struct {
	Items []BatchSanitizeItem `json:"items"`
}

type BatchSanitizeItem struct {
	ID               string `json:"id,omitempty"`
	Prompt           string `json:"prompt"`
	SessionID        string `json:"session_id,omitempty"`
	SanitizationMode string `json:"sanitization_mode,omitempty"`
	SLM              bool   `json:"slm,omitempty"`
}

type BatchSanitizeResponse struct {
	Results []BatchSanitizeItemResult `json:"results"`
	Summary BatchSanitizeSummary      `json:"summary"`
}

type BatchSanitizeItemResult struct {
	ID     string            `json:"id,omitempty"`
	OK     bool              `json:"ok"`
	Result *sanitizer.Result `json:"result,omitempty"`
	Error  *BatchItemError   `json:"error,omitempty"`
}

type BatchItemError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BatchSanitizeSummary struct {
	Total            int     `json:"total"`
	Succeeded        int     `json:"succeeded"`
	Failed           int     `json:"failed"`
	Violations       int     `json:"violations"`
	Redacted         int     `json:"redacted"`
	Tokenized        int     `json:"tokenized"`
	ProcessingTimeMs float64 `json:"processing_time_ms"`
}

func (h *BatchSanitizeHandler) Handle(c *gin.Context) {
	started := time.Now()
	if h.metrics != nil {
		h.metrics.IncBatchRequests()
	}

	var req BatchSanitizeRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.limits.MaxBatchBytes)
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&req); err != nil {
		if h.metrics != nil {
			h.metrics.IncErrors()
		}
		c.JSON(statusForDecodeError(err), gin.H{"error": "invalid request"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if h.metrics != nil {
			h.metrics.IncErrors()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.Items) == 0 {
		if h.metrics != nil {
			h.metrics.IncErrors()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "items is required"})
		return
	}
	if len(req.Items) > h.limits.MaxBatchItems {
		if h.metrics != nil {
			h.metrics.IncErrors()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many items"})
		return
	}

	resp := BatchSanitizeResponse{
		Results: make([]BatchSanitizeItemResult, 0, len(req.Items)),
		Summary: BatchSanitizeSummary{Total: len(req.Items)},
	}
	slmEnabledCount := 0
	redactModeCount := 0
	tokenizeModeCount := 0

	for _, item := range req.Items {
		if item.SLM {
			slmEnabledCount++
		}
		switch item.SanitizationMode {
		case "redact":
			redactModeCount++
		case "tokenize":
			tokenizeModeCount++
		}

		result := h.processItem(c, item)
		resp.Results = append(resp.Results, result)
		if !result.OK {
			resp.Summary.Failed++
			continue
		}
		resp.Summary.Succeeded++
		resp.Summary.Violations += len(result.Result.Violations)
		resp.Summary.Redacted += result.Result.RedactedCount
		resp.Summary.Tokenized += result.Result.TokenizedCount
	}

	elapsed := time.Since(started)
	resp.Summary.ProcessingTimeMs = float64(elapsed.Microseconds()) / 1000.0
	if h.metrics != nil {
		h.metrics.AddBatchItems(int64(resp.Summary.Total))
		h.metrics.AddBatchItemFailures(int64(resp.Summary.Failed))
		h.metrics.AddBatchViolations(int64(resp.Summary.Violations))
		h.metrics.AddBatchLatency(elapsed)
	}

	logger.Info("Batch sanitization completed", logger.Fields{
		"batch_size":        resp.Summary.Total,
		"succeeded":         resp.Summary.Succeeded,
		"failed":            resp.Summary.Failed,
		"violations":        resp.Summary.Violations,
		"redacted":          resp.Summary.Redacted,
		"tokenized":         resp.Summary.Tokenized,
		"processing_ms":     resp.Summary.ProcessingTimeMs,
		"slm_enabled_items": slmEnabledCount,
		"redact_items":      redactModeCount,
		"tokenize_items":    tokenizeModeCount,
	})

	c.JSON(http.StatusOK, resp)
}

func (h *BatchSanitizeHandler) processItem(c *gin.Context, item BatchSanitizeItem) BatchSanitizeItemResult {
	itemResult := BatchSanitizeItemResult{ID: item.ID}
	if item.Prompt == "" {
		itemResult.Error = batchError("missing_prompt", "prompt is required")
		return itemResult
	}
	if len(item.Prompt) > h.limits.MaxPromptBytes {
		itemResult.Error = batchError("prompt_too_large", "prompt exceeds max_prompt_bytes")
		return itemResult
	}

	sessionID := item.SessionID
	if sessionID == "" {
		sessionID = tokenizer.GenerateSessionID()
	} else if err := tokenizer.ValidateSessionID(sessionID); err != nil {
		itemResult.Error = batchError("invalid_session_id", "invalid session_id")
		return itemResult
	}

	switch item.SanitizationMode {
	case "", "redact", "tokenize":
	default:
		itemResult.Error = batchError("invalid_sanitization_mode", "invalid sanitization_mode")
		return itemResult
	}

	ctx := slm.WithRequestEnabled(c.Request.Context(), item.SLM)
	ctx = sanitizer.WithRequestMode(ctx, item.SanitizationMode)
	result, err := h.sanitizer.SanitizeWithContext(ctx, item.Prompt, sessionID)
	if err != nil {
		if errors.Is(err, tokenizer.ErrSessionStoreUnavailable) || errors.Is(err, tokenizer.ErrSessionStoreConflict) {
			itemResult.Error = batchError("session_store_unavailable", "session store unavailable")
			return itemResult
		}
		itemResult.Error = batchError("sanitization_failed", "sanitization failed")
		return itemResult
	}

	itemResult.OK = true
	batchResult := *result
	batchResult.OriginalPrompt = ""
	itemResult.Result = &batchResult
	return itemResult
}

func normalizeBatchLimits(limits BatchSanitizeLimits) BatchSanitizeLimits {
	if limits.MaxBatchItems <= 0 {
		limits.MaxBatchItems = 100
	}
	if limits.MaxPromptBytes <= 0 {
		limits.MaxPromptBytes = 65536
	}
	if limits.MaxBatchBytes <= 0 {
		limits.MaxBatchBytes = 1048576
	}
	if limits.MaxBatchConcurrency <= 0 {
		limits.MaxBatchConcurrency = 1
	}
	return limits
}

func batchError(code, message string) *BatchItemError {
	return &BatchItemError{
		Code:    code,
		Message: message,
	}
}

func statusForDecodeError(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
