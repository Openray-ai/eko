package openai

import (
	"eko/internal/core/sanitizer"
	"eko/internal/proxy/common"
)

// Proxy handles OpenAI API proxying
type Proxy struct {
	*common.BaseProxy
}

// New creates a new OpenAI proxy
func New(s *sanitizer.Sanitizer, baseURL string, timeout int) *Proxy {
	return &Proxy{
		BaseProxy: common.NewBaseProxy(s, baseURL, timeout),
	}
}

// ChatCompletionRequest represents OpenAI chat completion request
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// TODO: Implement chat completions endpoint handler
// TODO: Implement streaming support
// TODO: Implement request sanitization
// TODO: Implement response header injection
