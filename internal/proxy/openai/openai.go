package openai

import (
	"bytes"
	"context"
	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"
	"eko/internal/helpers/logger"
	"eko/internal/proxy/common"
	"encoding/json"
	"errors"
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
	resolver   *tokenizer.Resolver
}

// New creates a new OpenAI proxy.
func New(s *sanitizer.Sanitizer, baseURL string, timeout int) *Proxy {
	return NewWithResolver(s, nil, baseURL, timeout)
}

// NewWithResolver creates a new OpenAI proxy with an optional resolver.
func NewWithResolver(s *sanitizer.Sanitizer, resolver *tokenizer.Resolver, baseURL string, timeout int) *Proxy {
	return &Proxy{
		BaseProxy: common.NewBaseProxy(s, baseURL, timeout),
		httpClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		resolver: resolver,
	}
}

// GetResolver returns the resolver used for response token replacement.
func (p *Proxy) GetResolver() *tokenizer.Resolver {
	return p.resolver
}

// ChatCompletionRequest represents OpenAI chat completion request
type ChatCompletionRequest struct {
	Model            string         `json:"model"`
	Messages         []Message      `json:"messages"`
	Stream           bool           `json:"stream,omitempty"`
	Temperature      *float64       `json:"temperature,omitempty"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	N                *int           `json:"n,omitempty"`
	Stop             any            `json:"stop,omitempty"`
	User             string         `json:"user,omitempty"`
	Tools            any            `json:"tools,omitempty"`
	ToolChoice       any            `json:"tool_choice,omitempty"`
	ResponseFormat   any            `json:"response_format,omitempty"`
	Seed             *int           `json:"seed,omitempty"`
	LogitBias        map[string]any `json:"logit_bias,omitempty"`
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
	TotalTokenized    int
}

type CreateResponseRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Stream             bool            `json:"stream,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	MaxOutputTokens    *int            `json:"max_output_tokens,omitempty"`
	TopP               *float64        `json:"top_p,omitempty"`
	Tools              any             `json:"tools,omitempty"`
	ToolChoice         any             `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool           `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Instructions       string          `json:"instructions,omitempty"`
	Store              *bool           `json:"store,omitempty"`
	Metadata           any             `json:"metadata,omitempty"`
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
	TotalTokenized int
}

const streamingSanitizationOverride = "streaming_not_supported"

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
			"error": map[string]any{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	sessionID := p.ensureSessionID(c)

	// Log incoming request
	logger.Info("Processing OpenAI chat completion request", logger.Fields{
		"model":         req.Model,
		"message_count": len(req.Messages),
		"streaming":     req.Stream,
		"session_id":    sessionID,
	})

	// Sanitize messages
	var sanitizationResult *SanitizationResult
	var err error
	if req.Stream {
		sanitizationResult, err = p.sanitizeMessagesRedact(req.Messages)
	} else {
		sanitizationResult, err = p.sanitizeMessages(c.Request.Context(), req.Messages, sessionID)
	}
	if err != nil {
		logger.Error("Sanitization failed", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(statusCodeForSanitizationError(err), gin.H{
			"error": map[string]any{
				"message": "Failed to sanitize request",
				"type":    "internal_error",
			},
		})
		return
	}

	// Replace original messages with sanitized ones
	req.Messages = sanitizationResult.SanitizedMessages

	// Handle streaming vs non-streaming
	if req.Stream {
		// === STREAMING MODE ===
		p.logStreamingSanitizationOverride(sessionID)
		setStreamingSanitizationHeaders(c)
		setupSSEHeaders(c)

		// Send violation event first if violations found
		if err := sendViolationSSEEvent(c, sanitizationResult.AllViolations, sanitizationResult.TotalRedacted); err != nil {
			logger.Error("Failed to send violation event", logger.Fields{
				"error": err.Error(),
			})
			return
		}

		// Forward request to OpenAI (streaming)
		resp, err := p.forwardToOpenAIStream(c, &req)
		if err != nil {
			logger.Error("Failed to forward streaming request to OpenAI", logger.Fields{
				"error": err.Error(),
			})
			// Can't send JSON error after SSE headers, send as SSE error event
			fmt.Fprintf(c.Writer, "data: {\"error\":\"Failed to communicate with OpenAI\"}\n\n")
			c.Writer.Flush()
			return
		}
		defer resp.Body.Close()

		// Stream response to client
		if err := streamResponseToClient(c, resp); err != nil {
			logger.Error("Failed to stream response", logger.Fields{
				"error": err.Error(),
			})
			return
		}

		// Calculate total processing time
		processingTime := time.Since(startTime).Milliseconds()

		logger.Info("OpenAI streaming request processed successfully", logger.Fields{
			"status_code":      resp.StatusCode,
			"violations_found": len(sanitizationResult.AllViolations),
			"redacted_count":   sanitizationResult.TotalRedacted,
			"processing_ms":    processingTime,
		})
	} else {
		// === NON-STREAMING MODE ===
		// Forward request to OpenAI
		response, statusCode, err := p.forwardToOpenAI(c, &req)
		if err != nil {
			logger.Error("Failed to forward request to OpenAI", logger.Fields{
				"error": err.Error(),
			})
			c.JSON(http.StatusBadGateway, gin.H{
				"error": map[string]any{
					"message": "Failed to communicate with OpenAI",
					"type":    "proxy_error",
				},
			})
			return
		}

		response, err = p.resolveResponseIfNeeded(c, response, sessionID, sanitizationResult.TotalTokenized > 0)
		if err != nil {
			logger.Error("Failed to resolve response", logger.Fields{
				"error":      err.Error(),
				"session_id": sessionID,
			})
			c.JSON(statusCodeForResolutionError(err), gin.H{
				"error": map[string]any{
					"message": "Failed to resolve tokenized response",
					"type":    "internal_error",
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
}

type sanitizeTextFunc func(text string) (string, []detector.Violation, int, int, error)

// sanitizeMessages sanitizes all text content in messages
func (p *Proxy) sanitizeMessages(ctx context.Context, messages []Message, sessionID string) (*SanitizationResult, error) {
	return p.sanitizeMessagesWith(messages, func(text string) (string, []detector.Violation, int, int, error) {
		result, err := p.GetSanitizer().SanitizeWithContext(ctx, text, sessionID)
		if err != nil {
			return "", nil, 0, 0, err
		}

		return result.SanitizedPrompt, result.Violations, result.RedactedCount, result.TokenizedCount, nil
	}, true)
}

func (p *Proxy) sanitizeMessagesRedact(messages []Message) (*SanitizationResult, error) {
	return p.sanitizeMessagesWith(messages, func(text string) (string, []detector.Violation, int, int, error) {
		result, err := p.GetSanitizer().Sanitize(text)
		if err != nil {
			return "", nil, 0, 0, err
		}

		return result.SanitizedPrompt, result.Violations, result.RedactedCount, 0, nil
	}, false)
}

func (p *Proxy) sanitizeMessagesWith(messages []Message, sanitize sanitizeTextFunc, includeTokenized bool) (*SanitizationResult, error) {
	sanitizedMessages := make([]Message, len(messages))
	allViolations := []detector.Violation{}
	totalRedacted := 0
	totalTokenized := 0

	for i, msg := range messages {
		sanitizedText, violations, redactedCount, tokenizedCount, err := sanitize(msg.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to sanitize message %d: %w", i, err)
		}

		sanitizedMessages[i] = Message{
			Role:    msg.Role,
			Content: sanitizedText,
		}

		if len(violations) > 0 {
			allViolations = append(allViolations, violations...)
			totalRedacted += redactedCount
			if includeTokenized {
				totalTokenized += tokenizedCount
			}
		}
	}

	return &SanitizationResult{
		SanitizedMessages: sanitizedMessages,
		AllViolations:     allViolations,
		TotalRedacted:     totalRedacted,
		TotalTokenized:    totalTokenized,
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
	c.Header("X-Eko-Sanitization-Mode", p.GetSanitizer().SanitizationMode())
	c.Header("X-Eko-Tokens-Issued", fmt.Sprintf("%d", result.TotalTokenized))

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
	c.Header("X-Eko-Sanitization-Mode", p.GetSanitizer().SanitizationMode())
	c.Header("X-Eko-Tokens-Issued", fmt.Sprintf("%d", result.TotalTokenized))

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

func (p *Proxy) logStreamingSanitizationOverride(sessionID string) {
	logger.Warn("Streaming tokenization not supported; using redaction", logger.Fields{
		"sanitization_mode": p.GetSanitizer().SanitizationMode(),
		"effective_mode":    "redact",
		"override":          streamingSanitizationOverride,
		"session_id":        sessionID,
	})
}

func setStreamingSanitizationHeaders(c *gin.Context) {
	c.Header("X-Eko-Sanitization-Mode", "redact")
	c.Header("X-Eko-Sanitization-Override", streamingSanitizationOverride)
}

func (p *Proxy) HandleResponse(c *gin.Context) {
	startTime := time.Now()

	var req CreateResponseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("Failed to parse OpenAI Responses request", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadRequest, gin.H{
			"error": map[string]any{
				"message": "Invalid request format",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	sessionID := p.ensureSessionID(c)

	// Log incoming request
	logger.Info("Processing OpenAI Responses API request", logger.Fields{
		"model":      req.Model,
		"streaming":  req.Stream,
		"session_id": sessionID,
	})

	// Sanitize input
	var sanitizationResult *ResponseSanitizationResult
	var err error
	if req.Stream {
		sanitizationResult, err = p.sanitizeResponseInputRedact(req.Input)
	} else {
		sanitizationResult, err = p.sanitizeResponseInput(c.Request.Context(), req.Input, sessionID)
	}
	if err != nil {
		logger.Error("Sanitization failed", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(statusCodeForSanitizationError(err), gin.H{
			"error": map[string]any{
				"message": "Failed to sanitize request",
				"type":    "internal_error",
			},
		})
		return
	}

	// Replace original input with sanitized version
	req.Input = sanitizationResult.SanitizedInput

	// Handle streaming vs non-streaming
	if req.Stream {
		p.logStreamingSanitizationOverride(sessionID)
		setStreamingSanitizationHeaders(c)
		setupSSEHeaders(c)

		if err := sendViolationSSEEvent(c, sanitizationResult.AllViolations, sanitizationResult.TotalRedacted); err != nil {
			logger.Error("Failed to send violation event", logger.Fields{
				"error": err.Error(),
			})
			return
		}

		// Forward request to OpenAI (streaming)
		resp, err := p.forwardResponseToOpenAIStream(c, &req)
		if err != nil {
			logger.Error("Failed to forward streaming request to OpenAI", logger.Fields{
				"error": err.Error(),
			})
			// Can't send JSON error after SSE headers, send as SSE error event
			fmt.Fprintf(c.Writer, "data: {\"error\":\"Failed to communicate with OpenAI\"}\n\n")
			c.Writer.Flush()
			return
		}
		defer resp.Body.Close()

		// Stream response to client
		if err := streamResponseToClient(c, resp); err != nil {
			logger.Error("Failed to stream response", logger.Fields{
				"error": err.Error(),
			})
			return
		}

		// Calculate total processing time
		processingTime := time.Since(startTime).Milliseconds()

		logger.Info("OpenAI Responses streaming request processed successfully", logger.Fields{
			"status_code":      resp.StatusCode,
			"violations_found": len(sanitizationResult.AllViolations),
			"redacted_count":   sanitizationResult.TotalRedacted,
			"processing_ms":    processingTime,
		})
		return
	}

	// Forward request to OpenAI
	response, statusCode, err := p.forwardResponseToOpenAI(c, &req)
	if err != nil {
		logger.Error("Failed to forward request to OpenAI", logger.Fields{
			"error": err.Error(),
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error": map[string]any{
				"message": "Failed to communicate with OpenAI",
				"type":    "proxy_error",
			},
		})
		return
	}

	response, err = p.resolveResponseIfNeeded(c, response, sessionID, sanitizationResult.TotalTokenized > 0)
	if err != nil {
		logger.Error("Failed to resolve response", logger.Fields{
			"error":      err.Error(),
			"session_id": sessionID,
		})
		c.JSON(statusCodeForResolutionError(err), gin.H{
			"error": map[string]any{
				"message": "Failed to resolve tokenized response",
				"type":    "internal_error",
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

func (p *Proxy) resolveResponseIfNeeded(c *gin.Context, response []byte, sessionID string, requireResolution bool) ([]byte, error) {
	if p.resolver == nil || p.GetSanitizer().SanitizationMode() != "tokenize" {
		return response, nil
	}

	if !requireResolution {
		sessionExists, err := p.GetSanitizer().SessionExists(c.Request.Context(), sessionID)
		if err != nil {
			c.Header("X-Eko-Resolve-Status", resolveStatusFromError(err))
			logger.Warn("Failed to determine session state before resolution", logger.Fields{
				"error":      err.Error(),
				"session_id": sessionID,
			})
			return nil, err
		}
		requireResolution = sessionExists
	}

	resolved, err := p.resolver.ResolveResponseWithContext(c.Request.Context(), response, sessionID)
	if err != nil {
		status := resolveStatusFromError(err)
		c.Header("X-Eko-Resolve-Status", status)
		logger.Warn("Resolution failed", logger.Fields{
			"error":      err.Error(),
			"session_id": sessionID,
		})
		if requireResolution {
			return nil, err
		}
		return response, nil
	}

	c.Header("X-Eko-Resolve-Status", "success")
	return resolved, nil
}

func resolveStatusFromError(err error) string {
	switch {
	case errors.Is(err, tokenizer.ErrSessionExpired):
		return "vault_expired"
	case errors.Is(err, tokenizer.ErrVaultNotFound):
		return "no_vault"
	default:
		return "error"
	}
}

func statusCodeForSanitizationError(err error) int {
	switch {
	case errors.Is(err, tokenizer.ErrSessionStoreUnavailable), errors.Is(err, tokenizer.ErrSessionStoreConflict):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func statusCodeForResolutionError(err error) int {
	switch {
	case errors.Is(err, tokenizer.ErrSessionStoreUnavailable), errors.Is(err, tokenizer.ErrSessionStoreConflict):
		return http.StatusServiceUnavailable
	case errors.Is(err, tokenizer.ErrVaultNotFound), errors.Is(err, tokenizer.ErrSessionExpired):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func (p *Proxy) ensureSessionID(c *gin.Context) string {
	sessionID := c.GetHeader("X-Eko-Session-ID")
	if sessionID == "" {
		sessionID = tokenizer.GenerateSessionID()
	} else if err := tokenizer.ValidateSessionID(sessionID); err != nil {
		sessionID = tokenizer.GenerateSessionID()
	}
	c.Header("X-Eko-Session-ID", sessionID)
	return sessionID
}

// sanitizeResponseInput sanitizes the input field (string or array)
func (p *Proxy) sanitizeResponseInput(ctx context.Context, inputRaw json.RawMessage, sessionID string) (*ResponseSanitizationResult, error) {
	return p.sanitizeResponseInputWith(inputRaw, func(text string) (string, []detector.Violation, int, int, error) {
		result, err := p.GetSanitizer().SanitizeWithContext(ctx, text, sessionID)
		if err != nil {
			return "", nil, 0, 0, err
		}

		return result.SanitizedPrompt, result.Violations, result.RedactedCount, result.TokenizedCount, nil
	}, true)
}

func (p *Proxy) sanitizeResponseInputRedact(inputRaw json.RawMessage) (*ResponseSanitizationResult, error) {
	return p.sanitizeResponseInputWith(inputRaw, func(text string) (string, []detector.Violation, int, int, error) {
		result, err := p.GetSanitizer().Sanitize(text)
		if err != nil {
			return "", nil, 0, 0, err
		}

		return result.SanitizedPrompt, result.Violations, result.RedactedCount, 0, nil
	}, false)
}

func (p *Proxy) sanitizeResponseInputWith(inputRaw json.RawMessage, sanitize sanitizeTextFunc, includeTokenized bool) (*ResponseSanitizationResult, error) {
	allViolations := []detector.Violation{}
	totalRedacted := 0
	totalTokenized := 0

	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		sanitizedText, violations, redactedCount, tokenizedCount, err := sanitize(inputStr)
		if err != nil {
			return nil, fmt.Errorf("failed to sanitize string input: %w", err)
		}

		if len(violations) > 0 {
			allViolations = append(allViolations, violations...)
			totalRedacted += redactedCount
			if includeTokenized {
				totalTokenized += tokenizedCount
			}
		}

		sanitizedInput, err := json.Marshal(sanitizedText)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal sanitized input: %w", err)
		}

		return &ResponseSanitizationResult{
			SanitizedInput: sanitizedInput,
			AllViolations:  allViolations,
			TotalRedacted:  totalRedacted,
			TotalTokenized: totalTokenized,
		}, nil
	}

	var messages []ResponseMessage
	if err := json.Unmarshal(inputRaw, &messages); err != nil {
		return nil, fmt.Errorf("input must be either a string or array of messages: %w", err)
	}

	for i := range messages {
		for j := range messages[i].Content {
			contentBlock := &messages[i].Content[j]

			if contentBlock.Type == "input_text" && contentBlock.Text != "" {
				sanitizedText, violations, redactedCount, tokenizedCount, err := sanitize(contentBlock.Text)
				if err != nil {
					return nil, fmt.Errorf("failed to sanitize content block: %w", err)
				}

				contentBlock.Text = sanitizedText

				if len(violations) > 0 {
					allViolations = append(allViolations, violations...)
					totalRedacted += redactedCount
					if includeTokenized {
						totalTokenized += tokenizedCount
					}
				}
			}
		}
	}

	sanitizedInput, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sanitized messages: %w", err)
	}

	return &ResponseSanitizationResult{
		SanitizedInput: sanitizedInput,
		AllViolations:  allViolations,
		TotalRedacted:  totalRedacted,
		TotalTokenized: totalTokenized,
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

// ============================================================================
// Streaming Support
// ============================================================================

// setupSSEHeaders sets up Server-Sent Events headers for streaming responses
func setupSSEHeaders(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
}

// ViolationEvent represents the violation information sent as initial SSE event
type ViolationEvent struct {
	Type            string               `json:"type"`
	ViolationsFound int                  `json:"violations_found"`
	RedactedCount   int                  `json:"redacted_count"`
	Summary         string               `json:"summary"`
	Details         []detector.Violation `json:"details,omitempty"`
}

const (
	violationEventTypeLegacy = "eko.violation_report"
	violationEventTypeV1     = "eko.violation"
)

// sendViolationSSEEvent sends violation information as the first SSE event
func sendViolationSSEEvent(c *gin.Context, violations []detector.Violation, redactedCount int) error {
	if len(violations) == 0 {
		return nil // No violations, skip event
	}

	// Build summary string
	violationTypes := make(map[string]int)
	for _, v := range violations {
		violationTypes[v.Type]++
	}

	var summaryParts []string
	for vType, count := range violationTypes {
		summaryParts = append(summaryParts, fmt.Sprintf("%s:%d", vType, count))
	}

	// Create violation event
	baseEvent := ViolationEvent{
		ViolationsFound: len(violations),
		RedactedCount:   redactedCount,
		Summary:         strings.Join(summaryParts, ","),
		Details:         violations,
	}

	// Backward-compatibility: emit the legacy type first, then the newer type.
	eventTypes := []string{violationEventTypeLegacy, violationEventTypeV1}
	for _, eventType := range eventTypes {
		baseEvent.Type = eventType

		eventJSON, err := json.Marshal(baseEvent)
		if err != nil {
			return fmt.Errorf("failed to marshal violation event: %w", err)
		}

		_, err = fmt.Fprintf(c.Writer, "data: %s\n\n", string(eventJSON))
		if err != nil {
			return fmt.Errorf("failed to write violation event: %w", err)
		}
	}

	// Flush immediately
	c.Writer.Flush()

	logger.Info("Sent violation SSE event", logger.Fields{
		"violations_found": len(violations),
		"redacted_count":   redactedCount,
		"types":            eventTypes,
	})

	return nil
}

// streamResponseToClient streams the HTTP response body to the client using io.Copy
func streamResponseToClient(c *gin.Context, resp *http.Response) error {
	// Copy response body to client (pass-through streaming)
	_, err := io.Copy(c.Writer, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to stream response: %w", err)
	}

	return nil
}

// forwardToOpenAIStream forwards the request to OpenAI and returns the response for streaming
// Caller is responsible for closing response body
func (p *Proxy) forwardToOpenAIStream(c *gin.Context, req *ChatCompletionRequest) (*http.Response, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	openaiURL := fmt.Sprintf("%s/chat/completions", p.GetBaseURL())
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", openaiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy relevant headers from original request
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Forward request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

// forwardResponseToOpenAIStream forwards the request to OpenAI Responses API and returns the response for streaming
// Caller is responsible for closing response body
func (p *Proxy) forwardResponseToOpenAIStream(c *gin.Context, req *CreateResponseRequest) (*http.Response, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	openaiURL := fmt.Sprintf("%s/responses", p.GetBaseURL())
	httpReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", openaiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy relevant headers from original request
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		httpReq.Header.Set("Authorization", authHeader)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Forward request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}
