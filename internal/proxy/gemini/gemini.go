package gemini

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"eko/internal/proxy/common"
)

type Adapter struct {
	provider common.ProviderConfig
}

func New(baseURL string, timeout int) *Adapter {
	return &Adapter{
		provider: common.ProviderConfig{
			Name:    common.ProviderGemini,
			BaseURL: baseURL,
			Timeout: timeout,
		},
	}
}

func (a *Adapter) Name() common.ProviderName {
	return common.ProviderGemini
}

func (a *Adapter) Capabilities() common.Capabilities {
	return common.Capabilities{
		ChatCompletions:       true,
		Responses:             true,
		SystemPrompts:         true,
		ResponseNormalization: true,
	}
}

func (a *Adapter) ChatCompletions(req common.RouteRequest) (*common.RouteResponse, error) {
	if req.Stream {
		return nil, common.Unsupported(common.ProviderGemini, req.Route, req.Model, "chat_streaming", "streaming normalization is not supported for Gemini")
	}
	body, err := chatToGenerateContent(req.Body)
	if err != nil {
		return nil, err
	}
	resp, err := common.ForwardJSON(req, a.provider, generateContentPath(req.Model), body)
	if err != nil || resp.StatusCode >= 400 {
		return resp, err
	}
	resp.Body, err = normalizeChat(req.Model, resp.Body)
	return resp, err
}

func (a *Adapter) Responses(req common.RouteRequest) (*common.RouteResponse, error) {
	if req.Stream {
		return nil, common.Unsupported(common.ProviderGemini, req.Route, req.Model, "responses_streaming", "streaming responses normalization is not supported for Gemini")
	}
	body, err := responseToGenerateContent(req.Body)
	if err != nil {
		return nil, err
	}
	resp, err := common.ForwardJSON(req, a.provider, generateContentPath(req.Model), body)
	if err != nil || resp.StatusCode >= 400 {
		return resp, err
	}
	resp.Body, err = normalizeResponse(req.Model, resp.Body)
	return resp, err
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	Tools       any       `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
	ResponseFmt any       `json:"response_format,omitempty"`
	LogitBias   any       `json:"logit_bias,omitempty"`
	Stop        any       `json:"stop,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func chatToGenerateContent(body []byte) ([]byte, error) {
	if err := common.RejectUnsupportedTopLevelFields(body, geminiChatFields, common.ProviderGemini, "/v1/chat/completions", "gemini_chat_translation"); err != nil {
		return nil, err
	}
	if err := rejectUnsupportedMessageFields(body); err != nil {
		return nil, err
	}
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.Tools != nil || req.ToolChoice != nil || req.ResponseFmt != nil || req.LogitBias != nil || req.Stop != nil {
		return nil, common.Unsupported(common.ProviderGemini, "/v1/chat/completions", req.Model, "gemini_chat_translation", "Gemini adapter does not support tools, response_format, logit_bias, or stop yet")
	}
	return buildGenerateContent(req.Messages, req.MaxTokens, req.Temperature, req.TopP)
}

func responseToGenerateContent(body []byte) ([]byte, error) {
	if err := common.RejectUnsupportedTopLevelFields(body, geminiResponseFields, common.ProviderGemini, "/v1/responses", "responses_advanced_fields"); err != nil {
		return nil, err
	}
	var req struct {
		Model              string          `json:"model"`
		Input              json.RawMessage `json:"input"`
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
	if req.Tools != nil || req.ToolChoice != nil || req.ParallelToolCalls != nil || req.PreviousResponseID != "" || req.Instructions != "" || req.Store != nil || req.Metadata != nil || req.User != "" {
		return nil, common.Unsupported(common.ProviderGemini, "/v1/responses", req.Model, "responses_advanced_fields", "Gemini Responses adapter supports only text input and basic generation options")
	}
	messages, err := responseInputMessages(req.Input, req.Model)
	if err != nil {
		return nil, err
	}
	return buildGenerateContent(messages, req.MaxOutputTokens, req.Temperature, req.TopP)
}

func generateContentPath(model string) string {
	return fmt.Sprintf("/models/%s:generateContent", url.PathEscape(model))
}

func rejectUnsupportedMessageFields(body []byte) error {
	var raw struct {
		Model    string                       `json:"model"`
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	allowed := map[string]struct{}{
		"role":    {},
		"content": {},
	}
	for _, msg := range raw.Messages {
		for field := range msg {
			if _, ok := allowed[field]; !ok {
				return common.Unsupported(common.ProviderGemini, "/v1/chat/completions", raw.Model, "gemini_chat_translation", fmt.Sprintf("Gemini adapter does not support message field %q", field))
			}
		}
	}
	return nil
}

var geminiChatFields = map[string]struct{}{
	"model":           {},
	"messages":        {},
	"stream":          {},
	"temperature":     {},
	"max_tokens":      {},
	"top_p":           {},
	"tools":           {},
	"tool_choice":     {},
	"response_format": {},
	"logit_bias":      {},
	"stop":            {},
}

var geminiResponseFields = map[string]struct{}{
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

func buildGenerateContent(messages []message, maxTokens *int, temperature, topP *float64) ([]byte, error) {
	systemParts := []map[string]string{}
	contents := []map[string]any{}
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if msg.Content != "" {
				systemParts = append(systemParts, map[string]string{"text": msg.Content})
			}
		case "user", "assistant":
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			contents = append(contents, map[string]any{
				"role":  role,
				"parts": []map[string]string{{"text": msg.Content}},
			})
		default:
			return nil, common.Unsupported(common.ProviderGemini, "/v1/chat/completions", "", "gemini_roles", "Gemini adapter supports only system, user, and assistant roles")
		}
	}
	out := map[string]any{"contents": contents}
	if len(systemParts) > 0 {
		out["systemInstruction"] = map[string]any{"parts": systemParts}
	}
	if maxTokens != nil || temperature != nil || topP != nil {
		gen := map[string]any{}
		if maxTokens != nil {
			gen["maxOutputTokens"] = *maxTokens
		}
		if temperature != nil {
			gen["temperature"] = *temperature
		}
		if topP != nil {
			gen["topP"] = *topP
		}
		out["generationConfig"] = gen
	}
	return json.Marshal(out)
}

func responseInputMessages(raw json.RawMessage, model string) ([]message, error) {
	input, err := common.ResponsesInputTextMessages(raw, common.ProviderGemini, model)
	if err != nil {
		return nil, err
	}
	messages := make([]message, 0, len(input))
	for _, msg := range input {
		messages = append(messages, message{Role: msg.Role, Content: msg.Content})
	}
	return messages, nil
}

func normalizeChat(model string, body []byte) ([]byte, error) {
	text, err := textFromResponse(body)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"id":      fmt.Sprintf("chatcmpl_eko_%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index": 0,
			"message": map[string]string{
				"role":    "assistant",
				"content": text,
			},
			"finish_reason": "stop",
		}},
	})
}

func normalizeResponse(model string, body []byte) ([]byte, error) {
	text, err := textFromResponse(body)
	if err != nil {
		return nil, err
	}
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

func textFromResponse(body []byte) (string, error) {
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	parts := []string{}
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("Gemini response did not contain text content")
	}
	return strings.Join(parts, "\n"), nil
}
