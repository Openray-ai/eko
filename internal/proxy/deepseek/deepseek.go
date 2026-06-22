package deepseek

import (
	"encoding/json"
	"fmt"
	"time"

	"eko/internal/proxy/common"
)

type Adapter struct {
	provider common.ProviderConfig
}

func New(baseURL string, timeout int) *Adapter {
	return &Adapter{
		provider: common.ProviderConfig{
			Name:    common.ProviderDeepSeek,
			BaseURL: baseURL,
			Timeout: timeout,
		},
	}
}

func (a *Adapter) Name() common.ProviderName {
	return common.ProviderDeepSeek
}

func (a *Adapter) Capabilities() common.Capabilities {
	return common.Capabilities{
		ChatCompletions:       true,
		Responses:             true,
		Streaming:             true,
		ChatStreaming:         true,
		SystemPrompts:         true,
		ResponseNormalization: true,
		TokenResolution:       true,
	}
}

func (a *Adapter) ChatCompletions(req common.RouteRequest) (*common.RouteResponse, error) {
	if req.Stream {
		return common.ForwardStream(req, a.provider, "/chat/completions", req.Body)
	}
	return common.ForwardJSON(req, a.provider, "/chat/completions", req.Body)
}

func (a *Adapter) Responses(req common.RouteRequest) (*common.RouteResponse, error) {
	body, err := responseToChat(req.Body)
	if err != nil {
		return nil, err
	}
	resp, err := common.ForwardJSON(req, a.provider, "/chat/completions", body)
	if err != nil || resp.StatusCode >= 400 {
		return resp, err
	}
	resp.Body, err = normalizeResponse(req.Model, resp.Body)
	return resp, err
}

func responseToChat(body []byte) ([]byte, error) {
	if err := common.RejectUnsupportedTopLevelFields(body, deepseekResponseFields, common.ProviderDeepSeek, "/v1/responses", "responses_advanced_fields"); err != nil {
		return nil, err
	}
	var req struct {
		Model              string          `json:"model"`
		Input              json.RawMessage `json:"input"`
		Stream             bool            `json:"stream,omitempty"`
		MaxOutputTokens    *int            `json:"max_output_tokens,omitempty"`
		Temperature        *float64        `json:"temperature,omitempty"`
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
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.Stream {
		return nil, common.Unsupported(common.ProviderDeepSeek, "/v1/responses", req.Model, "responses_streaming", "streaming responses normalization is not supported for DeepSeek")
	}
	if req.Tools != nil || req.ToolChoice != nil || req.ParallelToolCalls != nil || req.PreviousResponseID != "" || req.Instructions != "" || req.Store != nil || req.Metadata != nil || req.User != "" {
		return nil, common.Unsupported(common.ProviderDeepSeek, "/v1/responses", req.Model, "responses_advanced_fields", "DeepSeek Responses adapter supports only text input and basic generation options")
	}
	messages, err := common.ResponsesInputTextMessages(req.Input, common.ProviderDeepSeek, req.Model)
	if err != nil {
		return nil, err
	}
	outMessages := make([]map[string]string, 0, len(messages))
	for _, msg := range messages {
		outMessages = append(outMessages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	out := map[string]any{
		"model":    req.Model,
		"messages": outMessages,
	}
	if req.MaxOutputTokens != nil {
		out["max_tokens"] = *req.MaxOutputTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		out["top_p"] = *req.TopP
	}
	return json.Marshal(out)
}

var deepseekResponseFields = map[string]struct{}{
	"model":                {},
	"input":                {},
	"stream":               {},
	"max_output_tokens":    {},
	"temperature":          {},
	"top_p":                {},
	"tools":                {},
	"tool_choice":          {},
	"parallel_tool_calls":  {},
	"previous_response_id": {},
	"instructions":         {},
	"store":                {},
	"metadata":             {},
	"user":                 {},
}

func normalizeResponse(model string, body []byte) ([]byte, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("DeepSeek response did not contain choices")
	}
	text := resp.Choices[0].Message.Content
	return json.Marshal(map[string]any{
		"id":      fmt.Sprintf("resp_eko_%d", time.Now().UnixNano()),
		"object":  "response",
		"created": time.Now().Unix(),
		"model":   model,
		"output": []map[string]any{{
			"type": "message",
			"role": "assistant",
			"content": []map[string]string{{
				"type": "output_text",
				"text": text,
			}},
		}},
		"output_text": text,
		"status":      "completed",
	})
}
