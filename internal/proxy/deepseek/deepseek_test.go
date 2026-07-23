package deepseek

import (
	"strings"
	"testing"
)

func TestResponseToChatBuildsOpenAICompatiblePayload(t *testing.T) {
	body, err := responseToChat([]byte(`{"model":"deepseek-chat","input":"hello"}`))
	if err != nil {
		t.Fatalf("responseToChat failed: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"model":"deepseek-chat"`, `"role":"user"`, `"content":"hello"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %s in %s", want, text)
		}
	}
}

func TestResponseToChatSupportsInputTextMessageArrays(t *testing.T) {
	body, err := responseToChat([]byte(`{"model":"deepseek-chat","input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_text","text":"world"}]}]}`))
	if err != nil {
		t.Fatalf("responseToChat failed: %v", err)
	}
	if !strings.Contains(string(body), `"content":"hello\nworld"`) {
		t.Fatalf("expected joined input_text content, got %s", body)
	}
}

func TestResponseToChatPreservesInputRoles(t *testing.T) {
	body, err := responseToChat([]byte(`{"model":"deepseek-chat","input":[{"role":"system","content":[{"type":"input_text","text":"be precise"}]},{"role":"assistant","content":[{"type":"input_text","text":"previous"}]},{"role":"user","content":[{"type":"input_text","text":"next"}]}]}`))
	if err != nil {
		t.Fatalf("responseToChat failed: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"role":"system"`, `"content":"be precise"`, `"role":"assistant"`, `"content":"previous"`, `"role":"user"`, `"content":"next"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %s in %s", want, text)
		}
	}
}

func TestResponseToChatMapsBasicGenerationOptions(t *testing.T) {
	body, err := responseToChat([]byte(`{"model":"deepseek-chat","input":"hello","max_output_tokens":123,"temperature":0.2,"top_p":0.9}`))
	if err != nil {
		t.Fatalf("responseToChat failed: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"max_tokens":123`, `"temperature":0.2`, `"top_p":0.9`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %s in %s", want, text)
		}
	}
}

func TestResponseToChatRejectsNonTextMessageArrays(t *testing.T) {
	_, err := responseToChat([]byte(`{"model":"deepseek-chat","input":[{"role":"user","content":[{"type":"input_image","image_url":"x"}]}]}`))
	if err == nil {
		t.Fatal("expected non-text input block to be rejected")
	}
}

func TestResponseToChatRejectsStreaming(t *testing.T) {
	_, err := responseToChat([]byte(`{"model":"deepseek-chat","stream":true,"input":"hello"}`))
	if err == nil {
		t.Fatal("expected streaming unsupported error")
	}
}

func TestResponseToChatRejectsUnknownTopLevelFields(t *testing.T) {
	_, err := responseToChat([]byte(`{"model":"deepseek-chat","input":"hello","include":["usage"]}`))
	if err == nil {
		t.Fatal("expected unsupported unknown Responses field error")
	}
}

func TestResponseToChatRejectsUnknownNestedInputFields(t *testing.T) {
	tests := []string{
		`{"model":"deepseek-chat","input":[{"id":"msg_1","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`,
		`{"model":"deepseek-chat","input":[{"role":"user","content":[{"type":"input_text","text":"hello","annotations":[]}]}]}`,
	}
	for _, body := range tests {
		_, err := responseToChat([]byte(body))
		if err == nil {
			t.Fatalf("expected unsupported nested Responses input field error for %s", body)
		}
	}
}

func TestNormalizeResponseOutputsResponsesShape(t *testing.T) {
	body, err := normalizeResponse("deepseek-chat", []byte(`{"choices":[{"message":{"content":"done"}}]}`))
	if err != nil {
		t.Fatalf("normalizeResponse failed: %v", err)
	}
	if !strings.Contains(string(body), `"object":"response"`) || !strings.Contains(string(body), `"output_text":"done"`) {
		t.Fatalf("unexpected normalized response: %s", body)
	}
}

func TestNormalizeResponseRejectsMissingChoices(t *testing.T) {
	if _, err := normalizeResponse("deepseek-chat", []byte(`{"choices":[]}`)); err == nil {
		t.Fatal("expected missing choices to fail normalization")
	}
}
