package openai

import (
	"eko/internal/proxy/common"
)

type Adapter struct {
	provider common.ProviderConfig
}

func NewAdapter(baseURL string, timeout int) *Adapter {
	return &Adapter{
		provider: common.ProviderConfig{
			Name:    common.ProviderOpenAI,
			BaseURL: baseURL,
			Timeout: timeout,
		},
	}
}

func (a *Adapter) Name() common.ProviderName {
	return common.ProviderOpenAI
}

func (a *Adapter) Capabilities() common.Capabilities {
	return common.Capabilities{
		ChatCompletions:    true,
		Responses:          true,
		Streaming:          true,
		ChatStreaming:      true,
		ResponsesStreaming: true,
		SystemPrompts:      true,
		TokenResolution:    true,
	}
}

func (a *Adapter) ChatCompletions(req common.RouteRequest) (*common.RouteResponse, error) {
	if req.Stream {
		return common.ForwardStream(req, a.provider, "/chat/completions", req.Body)
	}
	return common.ForwardJSON(req, a.provider, "/chat/completions", req.Body)
}

func (a *Adapter) Responses(req common.RouteRequest) (*common.RouteResponse, error) {
	if req.Stream {
		return common.ForwardStream(req, a.provider, "/responses", req.Body)
	}
	return common.ForwardJSON(req, a.provider, "/responses", req.Body)
}
