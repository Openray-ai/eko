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

// ============================================================================
// Responses API Types
// ============================================================================

// CreateResponseRequest represents OpenAI Responses API request
type CreateResponseRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"` // Can be string or array of ResponseMessage
	Stream             bool            `json:"stream,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	MaxOutputTokens    *int            `json:"max_output_tokens,omitempty"`
	TopP               *float64        `json:"top_p,omitempty"`
	Tools              interface{}     `json:"tools,omitempty"`
	ToolChoice         interface{}     `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Instructions       string          `json:"instructions,omitempty"`
	Store              *bool           `json:"store,omitempty"`
	Metadata           interface{}     `json:"metadata,omitempty"`
	User               string          `json:"user,omitempty"`
}

// ResponseMessage represents a message in Responses API
type ResponseMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block in a message
type ContentBlock struct {
	Type     string `json:"type"` // input_text, input_image, output_text, etc.
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

// ResponseSanitizationResult holds sanitization results for Responses API
type ResponseSanitizationResult struct {
	SanitizedInput json.RawMessage // The sanitized input (string or array)
	AllViolations  []detector.Violation
	TotalRedacted  int
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

// addResponseViolationHeaders adds violation headers for Responses API
func (p *Proxy) addResponseViolationHeaders(c *gin.Context, result *ResponseSanitizationResult) {
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

// ============================================================================
// Responses API Handlers
// ============================================================================

// HandleResponse processes OpenAI Responses API requests with sanitization
func (p *Proxy) HandleResponse(c *gin.Context) {
	startTime := time.Now()

	// Parse incoming request
	var req CreateResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Failed to parse OpenAI Responses request", logger.Fields{
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
	logger.Info("Processing OpenAI Responses API request", logger.Fields{
		"model": req.Model,
	})

	// Sanitize input
	sanitizationResult, err := p.sanitizeResponseInput(req.Input)
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

	// Replace original input with sanitized version
	req.Input = sanitizationResult.SanitizedInput

	// Forward request to OpenAI
	response, statusCode, err := p.forwardResponseToOpenAI(c, &req)
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
	p.addResponseViolationHeaders(c, sanitizationResult)

	// Calculate total processing time
	processingTime := time.Since(startTime).Milliseconds()

	logger.Info("OpenAI Responses request processed successfully", logger.Fields{
		"status_code":      statusCode,
		"violations_found": len(sanitizationResult.AllViolations),
		"redacted_count":   sanitizationResult.TotalRedacted,
		"processing_ms":    processingTime,
	})

	// Return OpenAI response
	c.Data(statusCode, "application/json", response)
}

// sanitizeResponseInput sanitizes the input field (string or array)
func (p *Proxy) sanitizeResponseInput(inputRaw json.RawMessage) (*ResponseSanitizationResult, error) {
	allViolations := []detector.Violation{}
	totalRedacted := 0

	// Try to parse as string first
	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		// Input is a simple string
		result, err := p.GetSanitizer().Sanitize(inputStr)
		if err != nil {
			return nil, fmt.Errorf("failed to sanitize string input: %w", err)
		}

		// Collect violations
		if len(result.Violations) > 0 {
			allViolations = append(allViolations, result.Violations...)
			totalRedacted += result.RedactedCount
		}

		// Re-marshal sanitized string
		sanitizedInput, err := json.Marshal(result.SanitizedPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal sanitized input: %w", err)
		}

		return &ResponseSanitizationResult{
			SanitizedInput: sanitizedInput,
			AllViolations:  allViolations,
			TotalRedacted:  totalRedacted,
		}, nil
	}

	// Try to parse as array of ResponseMessage
	var messages []ResponseMessage
	if err := json.Unmarshal(inputRaw, &messages); err != nil {
		return nil, fmt.Errorf("input must be either a string or array of messages: %w", err)
	}

	// Sanitize all input_text content blocks in messages
	for i := range messages {
		for j := range messages[i].Content {
			contentBlock := &messages[i].Content[j]

			// Only sanitize input_text content blocks
			if contentBlock.Type == "input_text" && contentBlock.Text != "" {
				result, err := p.GetSanitizer().Sanitize(contentBlock.Text)
				if err != nil {
					return nil, fmt.Errorf("failed to sanitize content block: %w", err)
				}

				// Replace with sanitized text
				contentBlock.Text = result.SanitizedPrompt

				// Collect violations
				if len(result.Violations) > 0 {
					allViolations = append(allViolations, result.Violations...)
					totalRedacted += result.RedactedCount
				}
			}
			// Skip input_image and other non-text content types
		}
	}

	// Re-marshal sanitized messages array
	sanitizedInput, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sanitized messages: %w", err)
	}

	return &ResponseSanitizationResult{
		SanitizedInput: sanitizedInput,
		AllViolations:  allViolations,
		TotalRedacted:  totalRedacted,
	}, nil
}

// forwardResponseToOpenAI forwards the sanitized request to OpenAI Responses API
func (p *Proxy) forwardResponseToOpenAI(c *gin.Context, req *CreateResponseRequest) ([]byte, int, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	openaiURL := fmt.Sprintf("%s/responses", p.GetBaseURL())
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
