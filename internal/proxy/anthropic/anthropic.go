package anthropic

import (
	"eko/internal/core/sanitizer"
	"eko/internal/proxy/common"
)

// Proxy handles Anthropic API proxying
type Proxy struct {
	*common.BaseProxy
}

// New creates a new Anthropic proxy
func New(s *sanitizer.Sanitizer, baseURL string, timeout int) *Proxy {
	return &Proxy{
		BaseProxy: common.NewBaseProxy(s, baseURL, timeout),
	}
}

// MessagesRequest represents Anthropic messages request
type MessagesRequest struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
	Stream    bool            `json:"stream,omitempty"`
}

// Message represents a message in the conversation
type Message struct {
	Role    string        `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block (text, image, etc.)
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TODO: Implement messages endpoint handler
// TODO: Implement streaming support
// TODO: Handle multiple content blocks
// TODO: Preserve image content blocks
