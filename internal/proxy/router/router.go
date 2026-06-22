package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"
	"eko/internal/helpers/logger"
	"eko/internal/proxy/common"

	"github.com/gin-gonic/gin"
)

const streamingSanitizationOverride = "streaming_not_supported"

type Proxy struct {
	router    *ModelRouter
	adapters  map[common.ProviderName]common.Adapter
	sanitizer *sanitizer.Sanitizer
	resolver  *tokenizer.Resolver
}

func New(s *sanitizer.Sanitizer, resolver *tokenizer.Resolver, routing RoutingConfig, adapters []common.Adapter) *Proxy {
	adapterMap := make(map[common.ProviderName]common.Adapter, len(adapters))
	for _, adapter := range adapters {
		adapterMap[adapter.Name()] = adapter
	}
	return &Proxy{
		router:    NewModelRouter(routing),
		adapters:  adapterMap,
		sanitizer: s,
		resolver:  resolver,
	}
}

func (p *Proxy) HandleChatCompletion(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeProxyError(c, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "failed to read request body", Route: "/v1/chat/completions"})
		return
	}
	var meta struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream,omitempty"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		writeProxyError(c, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "invalid request format", Route: "/v1/chat/completions"})
		return
	}
	resolved, adapter, err := p.resolve("/v1/chat/completions", meta.Model)
	if err != nil {
		writeProxyError(c, err)
		return
	}
	sessionID := p.ensureSessionID(c)
	if err := p.validateCapabilities(adapter, resolved, meta.Stream, true); err != nil {
		writeProxyError(c, err)
		return
	}
	sanitizedBody, result, err := p.sanitizeChatBody(c, body, meta.Stream, sessionID)
	if err != nil {
		writeProxyError(c, routeSanitizationError(err, resolved))
		return
	}
	req := common.RouteRequest{
		Context:   c.Request.Context(),
		Route:     resolved.Route,
		Model:     resolved.Model,
		Stream:    meta.Stream,
		Body:      sanitizedBody,
		Headers:   c.Request.Header.Clone(),
		Query:     c.Request.URL.Query(),
		SessionID: sessionID,
	}

	logger.Info("Processing routed chat completion request", logger.Fields{
		"route":    resolved.Route,
		"model":    resolved.Model,
		"provider": resolved.Provider,
		"stream":   meta.Stream,
	})

	resp, err := adapter.ChatCompletions(req)
	if err != nil {
		writeProxyError(c, enrichProxyError(err, resolved))
		return
	}
	if meta.Stream {
		if p.writeStreamFailure(c, resp, result, resolved) {
			return
		}
		p.writeStream(c, resp, result, sessionID)
		return
	}
	if resp.StatusCode >= http.StatusBadRequest {
		p.addViolationHeaders(c, result)
		c.Data(resp.StatusCode, resp.ContentType, resp.Body)
		return
	}
	resp.Body, err = p.resolveResponseIfNeeded(c, resp.Body, sessionID, result.TotalTokenized > 0)
	if err != nil {
		writeProxyError(c, &common.ProxyError{Status: statusCodeForResolutionError(err), Type: "internal_error", Message: "failed to resolve tokenized response", Provider: resolved.Provider, Model: resolved.Model, Route: resolved.Route})
		return
	}
	p.addViolationHeaders(c, result)
	c.Data(resp.StatusCode, resp.ContentType, resp.Body)
}

func (p *Proxy) HandleResponse(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		writeProxyError(c, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "failed to read request body", Route: "/v1/responses"})
		return
	}
	var reqBody struct {
		Model  string          `json:"model"`
		Input  json.RawMessage `json:"input"`
		Stream bool            `json:"stream,omitempty"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		writeProxyError(c, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "invalid request format", Route: "/v1/responses"})
		return
	}
	resolved, adapter, err := p.resolve("/v1/responses", reqBody.Model)
	if err != nil {
		writeProxyError(c, err)
		return
	}
	sessionID := p.ensureSessionID(c)
	if err := p.validateCapabilities(adapter, resolved, reqBody.Stream, false); err != nil {
		writeProxyError(c, err)
		return
	}
	sanitizedBody, result, err := p.sanitizeResponseBody(c, body, reqBody.Stream, sessionID, resolved.Provider)
	if err != nil {
		writeProxyError(c, routeSanitizationError(err, resolved))
		return
	}
	req := common.RouteRequest{
		Context:   c.Request.Context(),
		Route:     resolved.Route,
		Model:     resolved.Model,
		Stream:    reqBody.Stream,
		Body:      sanitizedBody,
		Headers:   c.Request.Header.Clone(),
		Query:     c.Request.URL.Query(),
		SessionID: sessionID,
	}

	logger.Info("Processing routed Responses request", logger.Fields{
		"route":    resolved.Route,
		"model":    resolved.Model,
		"provider": resolved.Provider,
		"stream":   reqBody.Stream,
	})

	resp, err := adapter.Responses(req)
	if err != nil {
		writeProxyError(c, enrichProxyError(err, resolved))
		return
	}
	if reqBody.Stream {
		if p.writeStreamFailure(c, resp, result, resolved) {
			return
		}
		p.writeStream(c, resp, result, sessionID)
		return
	}
	if resp.StatusCode >= http.StatusBadRequest {
		p.addViolationHeaders(c, result)
		c.Data(resp.StatusCode, resp.ContentType, resp.Body)
		return
	}
	resp.Body, err = p.resolveResponseIfNeeded(c, resp.Body, sessionID, result.TotalTokenized > 0)
	if err != nil {
		writeProxyError(c, &common.ProxyError{Status: statusCodeForResolutionError(err), Type: "internal_error", Message: "failed to resolve tokenized response", Provider: resolved.Provider, Model: resolved.Model, Route: resolved.Route})
		return
	}
	p.addViolationHeaders(c, result)
	c.Data(resp.StatusCode, resp.ContentType, resp.Body)
}

func (p *Proxy) resolve(route, model string) (ResolvedRoute, common.Adapter, error) {
	resolved, err := p.router.Resolve(route, model)
	if err != nil {
		return ResolvedRoute{}, nil, err
	}
	adapter := p.adapters[resolved.Provider]
	if adapter == nil {
		return ResolvedRoute{}, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "model_route_error", Message: "model route resolved to an unavailable provider", Provider: resolved.Provider, Model: resolved.Model, Route: resolved.Route}
	}
	return resolved, adapter, nil
}

func (p *Proxy) validateCapabilities(adapter common.Adapter, resolved ResolvedRoute, stream bool, chat bool) error {
	caps := adapter.Capabilities()
	if chat && !caps.ChatCompletions {
		return common.Unsupported(resolved.Provider, resolved.Route, resolved.Model, "chat_completions", "provider does not support chat completions")
	}
	if !chat && !caps.Responses {
		return common.Unsupported(resolved.Provider, resolved.Route, resolved.Model, "responses", "provider does not support Responses")
	}
	if stream {
		capability := "chat_streaming"
		supported := caps.ChatStreaming
		if !chat {
			capability = "responses_streaming"
			supported = caps.ResponsesStreaming
		}
		if !supported {
			if caps.Streaming && chat {
				supported = true
			}
		}
		if !supported {
			return common.Unsupported(resolved.Provider, resolved.Route, resolved.Model, capability, "streaming normalization is not supported for this provider")
		}
	}
	return nil
}

type sanitizeTextFunc func(text string) (string, []detector.Violation, int, int, error)

type SanitizationResult struct {
	AllViolations  []detector.Violation
	TotalRedacted  int
	TotalTokenized int
}

func (p *Proxy) sanitizeChatBody(c *gin.Context, body []byte, stream bool, sessionID string) ([]byte, *SanitizationResult, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(payload["messages"], &messages); err != nil {
		return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "messages must be an array", Capability: "chat_messages"}
	}
	sanitize := p.tokenizingSanitizer(c, stream, sessionID)
	result := &SanitizationResult{}
	for i := range messages {
		contentRaw, ok := messages[i]["content"]
		if !ok || string(contentRaw) == "null" {
			if err := sanitizeChatMessageMetadata(messages[i], sanitize, result); err != nil {
				return nil, nil, err
			}
			continue
		}
		var content string
		if err := json.Unmarshal(contentRaw, &content); err != nil {
			return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "only string chat message content is supported for provider routing", Capability: "multimodal_chat_content"}
		}
		sanitized, violations, redacted, tokenized, err := sanitize(content)
		if err != nil {
			return nil, nil, err
		}
		encoded, err := json.Marshal(sanitized)
		if err != nil {
			return nil, nil, err
		}
		messages[i]["content"] = encoded
		result.AllViolations = append(result.AllViolations, violations...)
		result.TotalRedacted += redacted
		result.TotalTokenized += tokenized
		if err := sanitizeChatMessageMetadata(messages[i], sanitize, result); err != nil {
			return nil, nil, err
		}
	}
	encodedMessages, err := json.Marshal(messages)
	if err != nil {
		return nil, nil, err
	}
	payload["messages"] = encodedMessages
	out, err := json.Marshal(payload)
	return out, result, err
}

func sanitizeChatMessageMetadata(message map[string]json.RawMessage, sanitize sanitizeTextFunc, result *SanitizationResult) error {
	if raw, ok := message["function_call"]; ok && string(raw) != "null" {
		sanitized, err := sanitizeFunctionCallArguments(raw, sanitize, result)
		if err != nil {
			return err
		}
		message["function_call"] = sanitized
	}
	if raw, ok := message["tool_calls"]; ok && string(raw) != "null" {
		var calls []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &calls); err != nil {
			return &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "tool_calls must be an array", Capability: "chat_tool_calls"}
		}
		for i := range calls {
			functionRaw, ok := calls[i]["function"]
			if !ok || string(functionRaw) == "null" {
				continue
			}
			sanitized, err := sanitizeFunctionCallArguments(functionRaw, sanitize, result)
			if err != nil {
				return err
			}
			calls[i]["function"] = sanitized
		}
		encoded, err := json.Marshal(calls)
		if err != nil {
			return err
		}
		message["tool_calls"] = encoded
	}
	return nil
}

func sanitizeFunctionCallArguments(raw json.RawMessage, sanitize sanitizeTextFunc, result *SanitizationResult) (json.RawMessage, error) {
	var function map[string]json.RawMessage
	if err := json.Unmarshal(raw, &function); err != nil {
		return nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "function_call/function metadata must be an object", Capability: "chat_function_call"}
	}
	argumentsRaw, ok := function["arguments"]
	if !ok || string(argumentsRaw) == "null" {
		return raw, nil
	}
	var arguments string
	if err := json.Unmarshal(argumentsRaw, &arguments); err != nil {
		return nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "function arguments must be a string", Capability: "chat_function_arguments"}
	}
	sanitized, violations, redacted, tokenized, err := sanitize(arguments)
	if err != nil {
		return nil, err
	}
	encodedArguments, err := json.Marshal(sanitized)
	if err != nil {
		return nil, err
	}
	function["arguments"] = encodedArguments
	result.AllViolations = append(result.AllViolations, violations...)
	result.TotalRedacted += redacted
	result.TotalTokenized += tokenized
	return json.Marshal(function)
}

func (p *Proxy) sanitizeResponseBody(c *gin.Context, body []byte, stream bool, sessionID string, provider common.ProviderName) ([]byte, *SanitizationResult, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	input, ok := payload["input"]
	if !ok {
		return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "input is required", Capability: "responses_input"}
	}
	sanitizedInput, result, err := p.sanitizeResponseInput(c, input, stream, sessionID, provider)
	if err != nil {
		return nil, nil, err
	}
	payload["input"] = sanitizedInput
	out, err := json.Marshal(payload)
	return out, result, err
}

func (p *Proxy) sanitizeResponseInput(c *gin.Context, input json.RawMessage, stream bool, sessionID string, provider common.ProviderName) (json.RawMessage, *SanitizationResult, error) {
	sanitize := p.tokenizingSanitizer(c, stream, sessionID)
	result := &SanitizationResult{}
	var inputStr string
	if err := json.Unmarshal(input, &inputStr); err == nil {
		sanitized, violations, redacted, tokenized, err := sanitize(inputStr)
		if err != nil {
			return nil, nil, err
		}
		result.AllViolations = append(result.AllViolations, violations...)
		result.TotalRedacted += redacted
		result.TotalTokenized += tokenized
		encoded, err := json.Marshal(sanitized)
		return encoded, result, err
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(input, &messages); err != nil {
		return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "Responses input must be a string or input_text message array", Capability: "responses_input"}
	}
	for i := range messages {
		contentRaw, ok := messages[i]["content"]
		if !ok || string(contentRaw) == "null" {
			continue
		}
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(contentRaw, &blocks); err != nil {
			return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "Responses input must use content block arrays", Capability: "responses_input"}
		}
		for j := range blocks {
			var blockType string
			if err := json.Unmarshal(blocks[j]["type"], &blockType); err != nil {
				return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "Responses content blocks require a string type", Capability: "responses_input"}
			}
			if blockType != "input_text" {
				if provider == common.ProviderOpenAI {
					continue
				}
				return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "Responses routing supports only input_text blocks", Capability: "responses_multimodal"}
			}
			var text string
			if err := json.Unmarshal(blocks[j]["text"], &text); err != nil {
				return nil, nil, &common.ProxyError{Status: http.StatusBadRequest, Type: "unsupported_capability", Message: "Responses input_text blocks require string text", Capability: "responses_input_text"}
			}
			sanitized, violations, redacted, tokenized, err := sanitize(text)
			if err != nil {
				return nil, nil, err
			}
			encodedText, err := json.Marshal(sanitized)
			if err != nil {
				return nil, nil, err
			}
			blocks[j]["text"] = encodedText
			result.AllViolations = append(result.AllViolations, violations...)
			result.TotalRedacted += redacted
			result.TotalTokenized += tokenized
		}
		encodedContent, err := json.Marshal(blocks)
		if err != nil {
			return nil, nil, err
		}
		messages[i]["content"] = encodedContent
	}
	encoded, err := json.Marshal(messages)
	return encoded, result, err
}

func (p *Proxy) tokenizingSanitizer(c *gin.Context, stream bool, sessionID string) sanitizeTextFunc {
	if stream {
		return func(text string) (string, []detector.Violation, int, int, error) {
			result, err := p.sanitizer.SanitizeRedactWithContext(c.Request.Context(), text)
			if err != nil {
				return "", nil, 0, 0, err
			}
			return result.SanitizedPrompt, result.Violations, result.RedactedCount, 0, nil
		}
	}
	return func(text string) (string, []detector.Violation, int, int, error) {
		result, err := p.sanitizer.SanitizeWithContext(c.Request.Context(), text, sessionID)
		if err != nil {
			return "", nil, 0, 0, err
		}
		return result.SanitizedPrompt, result.Violations, result.RedactedCount, result.TokenizedCount, nil
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

func (p *Proxy) addViolationHeaders(c *gin.Context, result *SanitizationResult) {
	c.Header("X-Eko-Violations-Found", fmt.Sprintf("%d", len(result.AllViolations)))
	c.Header("X-Eko-Redacted-Count", fmt.Sprintf("%d", result.TotalRedacted))
	c.Header("X-Eko-Sanitization-Mode", p.sanitizer.SanitizationMode())
	c.Header("X-Eko-Tokens-Issued", fmt.Sprintf("%d", result.TotalTokenized))
	if len(result.AllViolations) > 0 {
		c.Header("X-Eko-Violation-Summary", violationSummary(result.AllViolations))
	}
}

func (p *Proxy) writeStream(c *gin.Context, resp *common.RouteResponse, result *SanitizationResult, sessionID string) {
	p.logStreamingSanitizationOverride(sessionID)
	c.Header("X-Eko-Sanitization-Mode", "redact")
	c.Header("X-Eko-Sanitization-Override", streamingSanitizationOverride)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	if len(result.AllViolations) > 0 {
		if err := writeViolationSSEEvents(c, result); err != nil {
			logger.Error("Failed to write violation SSE event", logger.Fields{
				"error": err.Error(),
			})
			return
		}
	}
	if resp == nil || resp.Stream == nil {
		return
	}
	defer resp.Stream.Body.Close()
	_, _ = io.Copy(c.Writer, resp.Stream.Body)
}

func (p *Proxy) writeStreamFailure(c *gin.Context, resp *common.RouteResponse, result *SanitizationResult, resolved ResolvedRoute) bool {
	if resp == nil || resp.Stream == nil {
		writeProxyError(c, &common.ProxyError{
			Status:   http.StatusBadGateway,
			Type:     "proxy_error",
			Message:  fmt.Sprintf("failed to communicate with %s provider", resolved.Provider),
			Provider: resolved.Provider,
			Model:    resolved.Model,
			Route:    resolved.Route,
		})
		return true
	}
	if resp.StatusCode < http.StatusBadRequest {
		return false
	}
	defer resp.Stream.Body.Close()
	body, err := io.ReadAll(resp.Stream.Body)
	if err != nil {
		writeProxyError(c, &common.ProxyError{
			Status:   http.StatusBadGateway,
			Type:     "proxy_error",
			Message:  fmt.Sprintf("failed to read %s provider error response", resolved.Provider),
			Provider: resolved.Provider,
			Model:    resolved.Model,
			Route:    resolved.Route,
		})
		return true
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = resp.Stream.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "application/json"
	}
	p.addViolationHeaders(c, result)
	c.Data(resp.StatusCode, contentType, body)
	return true
}

type violationEvent struct {
	Type            string               `json:"type"`
	ViolationsFound int                  `json:"violations_found"`
	RedactedCount   int                  `json:"redacted_count"`
	Summary         string               `json:"summary"`
	Details         []detector.Violation `json:"details,omitempty"`
}

func writeViolationSSEEvents(c *gin.Context, result *SanitizationResult) error {
	base := violationEvent{
		ViolationsFound: len(result.AllViolations),
		RedactedCount:   result.TotalRedacted,
		Summary:         violationSummary(result.AllViolations),
		Details:         result.AllViolations,
	}
	for _, eventType := range []string{"eko.violation_report", "eko.violation"} {
		base.Type = eventType
		payload, err := json.Marshal(base)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
			return err
		}
	}
	c.Writer.Flush()
	return nil
}

func violationSummary(violations []detector.Violation) string {
	violationTypes := make(map[string]int)
	for _, violation := range violations {
		violationTypes[violation.Type]++
	}
	parts := make([]string, 0, len(violationTypes))
	for violationType, count := range violationTypes {
		parts = append(parts, fmt.Sprintf("%s:%d", violationType, count))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (p *Proxy) logStreamingSanitizationOverride(sessionID string) {
	logger.Warn("Streaming tokenization not supported; using redaction", logger.Fields{
		"sanitization_mode": p.sanitizer.SanitizationMode(),
		"effective_mode":    "redact",
		"override":          streamingSanitizationOverride,
		"session_id":        sessionID,
	})
}

func (p *Proxy) resolveResponseIfNeeded(c *gin.Context, response []byte, sessionID string, requireResolution bool) ([]byte, error) {
	if p.resolver == nil || p.sanitizer.SanitizationMode() != "tokenize" {
		return response, nil
	}
	if !requireResolution {
		sessionExists, err := p.sanitizer.SessionExists(c.Request.Context(), sessionID)
		if err != nil {
			c.Header("X-Eko-Resolve-Status", resolveStatusFromError(err))
			return nil, err
		}
		requireResolution = sessionExists
	}
	resolved, err := p.resolver.ResolveResponseWithContext(c.Request.Context(), response, sessionID)
	if err != nil {
		c.Header("X-Eko-Resolve-Status", resolveStatusFromError(err))
		if requireResolution {
			return nil, err
		}
		return response, nil
	}
	c.Header("X-Eko-Resolve-Status", "success")
	return resolved, nil
}

func writeProxyError(c *gin.Context, err error) {
	pe, ok := common.AsProxyError(err)
	if !ok {
		pe = &common.ProxyError{Status: http.StatusInternalServerError, Type: "internal_error", Message: "internal proxy error"}
	}
	c.JSON(pe.Status, gin.H{"error": map[string]any{
		"message":    pe.Message,
		"type":       pe.Type,
		"route":      pe.Route,
		"model":      pe.Model,
		"provider":   pe.Provider,
		"capability": pe.Capability,
	}})
}

func routeSanitizationError(err error, route ResolvedRoute) error {
	if pe, ok := common.AsProxyError(err); ok {
		pe.Provider = route.Provider
		pe.Model = route.Model
		pe.Route = route.Route
		return pe
	}
	return &common.ProxyError{Status: statusCodeForSanitizationError(err), Type: "internal_error", Message: "failed to sanitize request", Provider: route.Provider, Model: route.Model, Route: route.Route}
}

func enrichProxyError(err error, route ResolvedRoute) error {
	if pe, ok := common.AsProxyError(err); ok {
		if pe.Provider == "" {
			pe.Provider = route.Provider
		}
		if pe.Model == "" {
			pe.Model = route.Model
		}
		if pe.Route == "" {
			pe.Route = route.Route
		}
		return pe
	}
	return common.ProxyFailure(route.Provider, route.Route, route.Model, err)
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
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}

func ModelFamily(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "claude"
	case strings.HasPrefix(model, "gemini-"):
		return "gemini"
	case strings.HasPrefix(model, "deepseek-"):
		return "deepseek"
	case strings.HasPrefix(model, "gpt-"), strings.HasPrefix(model, "o"):
		return "openai"
	default:
		return "custom"
	}
}
