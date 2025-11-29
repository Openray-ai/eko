package openai

import (
	"bytes"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/helpers/logger"
	"eko/internal/proxy/common"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Proxy handles OpenAI API proxying
type Proxy struct {
	*common.BaseProxy
	httpClient *http.Client
}

// New creates a new OpenAI proxy
func New(s *sanitizer.Sanitizer, baseURL string, timeout int) *Proxy {
	return &Proxy{
		BaseProxy: common.NewBaseProxy(s, baseURL, timeout),
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// ChatCompletionRequest represents OpenAI chat completion request
type ChatCompletionRequest struct {
	Model            string                 `json:"model"`
	Messages         []Message              `json:"messages"`
	Stream           bool                   `json:"stream,omitempty"`
	Temperature      *float64               `json:"temperature,omitempty"`
	MaxTokens        *int                   `json:"max_tokens,omitempty"`
	TopP             *float64               `json:"top_p,omitempty"`
	FrequencyPenalty *float64               `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64               `json:"presence_penalty,omitempty"`
	N                *int                   `json:"n,omitempty"`
	Stop             interface{}            `json:"stop,omitempty"`
	User             string                 `json:"user,omitempty"`
	Tools            interface{}            `json:"tools,omitempty"`
	ToolChoice       interface{}            `json:"tool_choice,omitempty"`
	ResponseFormat   interface{}            `json:"response_format,omitempty"`
	Seed             *int                   `json:"seed,omitempty"`
	LogitBias        map[string]interface{} `json:"logit_bias,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SanitizationResult holds the result of sanitizing messages
type SanitizationResult struct {
	SanitizedMessages []Message
	AllViolations     []detector.Violation
	TotalRedacted     int
}

// HandleChatCompletion processes OpenAI chat completion requests with sanitization
func (p *Proxy) HandleChatCompletion(c *gin.Context) {
	startTime := time.Now()

	// Parse incoming request
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Failed to parse OpenAI request", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Check for streaming (not supported yet)
	if req.Stream {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]interface{}{
				"message": "Streaming is not supported",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Log incoming request
	logger.Info("Processing OpenAI chat completion request", logger.Fields{
		"model":         req.Model,
		"message_count": len(req.Messages),
	})

	// Sanitize messages
	sanitizationResult, err := p.sanitizeMessages(req.Messages)
	if err != nil {
		logger.Error("Sanitization failed", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": map[string]interface{}{
				"message": "Failed to sanitize request",
				"type":    "internal_error",
			},
		})
		return
	}

	logger.Info("Sanitization result", logger.Fields{
		"sanitized_messages": sanitizationResult.SanitizedMessages,
		"all_violations":     sanitizationResult.AllViolations,
		"total_redacted":     sanitizationResult.TotalRedacted,
	})

	// Replace original messages with sanitized ones
	req.Messages = sanitizationResult.SanitizedMessages

	// Forward request to OpenAI
	response, statusCode, err := p.forwardToOpenAI(c, &req)
	if err != nil {
		logger.Error("Failed to forward request to OpenAI", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error": map[string]interface{}{
				"message": "Failed to communicate with OpenAI",
				"type":    "proxy_error",
			},
		})
		return
	}

	// Add violation headers
	p.addViolationHeaders(c, sanitizationResult)

	// Calculate total processing time
	processingTime := time.Since(startTime).Milliseconds()

	logger.Info("OpenAI request processed successfully", logger.Fields{
		"status_code":      statusCode,
		"violations_found": len(sanitizationResult.AllViolations),
		"redacted_count":   sanitizationResult.TotalRedacted,
		"processing_ms":    processingTime,
	})

	// Return OpenAI response
	c.Data(statusCode, "application/json", response)
}

// sanitizeMessages sanitizes all text content in messages
func (p *Proxy) sanitizeMessages(messages []Message) (*SanitizationResult, error) {
	sanitizedMessages := make([]Message, len(messages))
	allViolations := []detector.Violation{}
	totalRedacted := 0

	for i, msg := range messages {
		// Sanitize the content
		result, err := p.GetSanitizer().Sanitize(msg.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to sanitize message %d: %w", i, err)
		}

		// Create sanitized message
		sanitizedMessages[i] = Message{
			Role:    msg.Role,
			Content: result.SanitizedPrompt,
		}

		// Collect violations
		if len(result.Violations) > 0 {
			allViolations = append(allViolations, result.Violations...)
			totalRedacted += result.RedactedCount
		}
	}

	return &SanitizationResult{
		SanitizedMessages: sanitizedMessages,
		AllViolations:     allViolations,
		TotalRedacted:     totalRedacted,
	}, nil
}

// forwardToOpenAI forwards the sanitized request to OpenAI API
func (p *Proxy) forwardToOpenAI(c *gin.Context, req *ChatCompletionRequest) ([]byte, int, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	openaiURL := fmt.Sprintf("%s/chat/completions", p.GetBaseURL())
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", openaiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy relevant headers from original request
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Forward request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// addViolationHeaders adds custom headers with violation information
func (p *Proxy) addViolationHeaders(c *gin.Context, result *SanitizationResult) {
	c.Header("X-Eko-Violations-Found", fmt.Sprintf("%d", len(result.AllViolations)))
	c.Header("X-Eko-Redacted-Count", fmt.Sprintf("%d", result.TotalRedacted))

	if len(result.AllViolations) > 0 {
		// Create a summary of violation types
		violationTypes := make(map[string]int)
		for _, v := range result.AllViolations {
			violationTypes[v.Type]++
		}

		// Build summary string
		var summary []string
		for vType, count := range violationTypes {
			summary = append(summary, fmt.Sprintf("%s:%d", vType, count))
		}
		c.Header("X-Eko-Violation-Summary", strings.Join(summary, ","))
	}
}
