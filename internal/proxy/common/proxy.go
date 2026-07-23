package common

import (
	"context"
	"eko/internal/core/sanitizer"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Proxy defines the interface for provider-specific proxies
type Proxy interface {
	// Handle processes and forwards requests to the provider
	Handle(request interface{}) (interface{}, error)
}

// BaseProxy contains common proxy functionality
type BaseProxy struct {
	sanitizer *sanitizer.Sanitizer
	baseURL   string
	timeout   int
}

// NewBaseProxy creates a new base proxy
func NewBaseProxy(s *sanitizer.Sanitizer, baseURL string, timeout int) *BaseProxy {
	return &BaseProxy{
		sanitizer: s,
		baseURL:   baseURL,
		timeout:   timeout,
	}
}

// GetSanitizer returns the sanitizer instance
func (bp *BaseProxy) GetSanitizer() *sanitizer.Sanitizer {
	return bp.sanitizer
}

// GetBaseURL returns the base URL for the provider
func (bp *BaseProxy) GetBaseURL() string {
	return bp.baseURL
}

// GetTimeout returns the timeout in seconds
func (bp *BaseProxy) GetTimeout() int {
	return bp.timeout
}

// ProxyResponse represents a generic proxy response
type ProxyResponse struct {
	Data             interface{} `json:"data"`
	ViolationsFound  int         `json:"violations_found"`
	ViolationDetails string      `json:"violation_details,omitempty"`
}

type ProviderName string

const (
	ProviderOpenAI    ProviderName = "openai"
	ProviderAnthropic ProviderName = "anthropic"
	ProviderGemini    ProviderName = "gemini"
	ProviderDeepSeek  ProviderName = "deepseek"
)

type ProviderConfig struct {
	Name    ProviderName
	BaseURL string
	Timeout int
}

type Capabilities struct {
	ChatCompletions       bool
	Responses             bool
	Streaming             bool
	ChatStreaming         bool
	ResponsesStreaming    bool
	Tools                 bool
	JSONMode              bool
	Multimodal            bool
	SystemPrompts         bool
	ResponseNormalization bool
	TokenResolution       bool
}

type RouteRequest struct {
	Context   context.Context
	Route     string
	Model     string
	Stream    bool
	Body      []byte
	Headers   http.Header
	Query     url.Values
	SessionID string
}

type RouteResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
	Stream      *http.Response
}

type Adapter interface {
	Name() ProviderName
	Capabilities() Capabilities
	ChatCompletions(RouteRequest) (*RouteResponse, error)
	Responses(RouteRequest) (*RouteResponse, error)
}

type TextMessage struct {
	Role    string
	Content string
}

type ProxyError struct {
	Status     int
	Type       string
	Message    string
	Provider   ProviderName
	Model      string
	Route      string
	Capability string
}

func (e *ProxyError) Error() string {
	return e.Message
}

func AsProxyError(err error) (*ProxyError, bool) {
	var pe *ProxyError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}

func Unsupported(provider ProviderName, route, model, capability, message string) *ProxyError {
	return &ProxyError{
		Status:     http.StatusBadRequest,
		Type:       "unsupported_capability",
		Message:    message,
		Provider:   provider,
		Model:      model,
		Route:      route,
		Capability: capability,
	}
}

func RejectUnsupportedTopLevelFields(body []byte, allowed map[string]struct{}, provider ProviderName, route, capability string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	model := ""
	if rawModel, ok := raw["model"]; ok {
		_ = json.Unmarshal(rawModel, &model)
	}
	for field := range raw {
		if _, ok := allowed[field]; !ok {
			return Unsupported(provider, route, model, capability, fmt.Sprintf("%s does not support field %q", provider, field))
		}
	}
	return nil
}

func ResponsesInputTextMessages(raw json.RawMessage, provider ProviderName, model string) ([]TextMessage, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []TextMessage{{Role: "user", Content: s}}, nil
	}

	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses adapter supports only string input or input_text message arrays", provider))
	}
	if len(messages) == 0 {
		return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses input must include at least one message", provider))
	}

	out := make([]TextMessage, 0, len(messages))
	for _, msg := range messages {
		for field := range msg {
			if field != "role" && field != "content" {
				return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses adapter does not support input message field %q", provider, field))
			}
		}

		var role string
		if rawRole, ok := msg["role"]; ok {
			if err := json.Unmarshal(rawRole, &role); err != nil {
				return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses input message role must be a string", provider))
			}
		} else {
			role = "user"
		}
		if role != "system" && role != "user" && role != "assistant" {
			return nil, Unsupported(provider, "/v1/responses", model, "responses_roles", fmt.Sprintf("%s Responses adapter supports only system, user, and assistant input roles", provider))
		}

		rawContent, ok := msg["content"]
		if !ok {
			return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses input message requires content", provider))
		}
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(rawContent, &blocks); err != nil {
			return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses adapter supports only input_text message arrays", provider))
		}
		if len(blocks) == 0 {
			return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses input message must include at least one content block", provider))
		}

		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			for field := range block {
				if field != "type" && field != "text" {
					return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s Responses adapter does not support input_text field %q", provider, field))
				}
			}
			var blockType string
			if err := json.Unmarshal(block["type"], &blockType); err != nil || blockType != "input_text" {
				return nil, Unsupported(provider, "/v1/responses", model, "responses_multimodal", fmt.Sprintf("%s Responses adapter supports only input_text blocks", provider))
			}
			rawText, ok := block["text"]
			if !ok {
				return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s input_text block requires text", provider))
			}
			var text string
			if err := json.Unmarshal(rawText, &text); err != nil {
				return nil, Unsupported(provider, "/v1/responses", model, "responses_input", fmt.Sprintf("%s input_text text must be a string", provider))
			}
			parts = append(parts, text)
		}
		out = append(out, TextMessage{Role: role, Content: strings.Join(parts, "\n")})
	}
	return out, nil
}
