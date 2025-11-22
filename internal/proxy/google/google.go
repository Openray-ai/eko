package google

import (
	"eko/internal/core/sanitizer"
	"eko/internal/proxy/common"
)

// Proxy handles Google AI API proxying
type Proxy struct {
	*common.BaseProxy
}

// New creates a new Google AI proxy
func New(s *sanitizer.Sanitizer, baseURL string, timeout int) *Proxy {
	return &Proxy{
		BaseProxy: common.NewBaseProxy(s, baseURL, timeout),
	}
}

// GenerateContentRequest represents Google AI generate content request
type GenerateContentRequest struct {
	Contents []Content `json:"contents"`
}

// Content represents content to be sent to the model
type Content struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

// Part represents a part of the content
type Part struct {
	Text string `json:"text"`
}

// TODO: Implement generate content endpoint handler
// TODO: Handle multi-turn conversations
// TODO: Preserve safety settings and generation config
