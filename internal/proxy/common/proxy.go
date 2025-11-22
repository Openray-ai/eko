package common

import (
	"eko/internal/core/sanitizer"
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

// ProxyResponse represents a generic proxy response
type ProxyResponse struct {
	Data              interface{} `json:"data"`
	ViolationsFound   int         `json:"violations_found"`
	ViolationDetails  string      `json:"violation_details,omitempty"`
}
