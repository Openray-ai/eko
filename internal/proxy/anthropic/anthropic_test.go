package anthropic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatToMessagesMapsSystemAndTextMessages(t *testing.T) {
	body, err := chatToMessages([]byte(`{"model":"claude-3","messages":[{"role":"system","content":"be precise"},{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("chatToMessages failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["system"] != "be precise" {
		t.Fatalf("system = %v, want be precise", got["system"])
	}
	if !strings.Contains(string(body), `"type":"text"`) || !strings.Contains(string(body), `"text":"hello"`) {
		t.Fatalf("expected text content block, got %s", body)
	}
}

func TestChatToMessagesRejectsUnsupportedTools(t *testing.T) {
	_, err := chatToMessages([]byte(`{"model":"claude-3","tools":[{"type":"function"}],"messages":[{"role":"user","content":"hello"}]}`))
	if err == nil {
		t.Fatal("expected unsupported tools error")
	}
}

func TestChatToMessagesRejectsUnknownTopLevelFields(t *testing.T) {
	_, err := chatToMessages([]byte(`{"model":"claude-3","presence_penalty":1,"messages":[{"role":"user","content":"hello"}]}`))
	if err == nil {
		t.Fatal("expected unsupported unknown field error")
	}
}

func TestChatToMessagesRejectsUnsupportedMessageFields(t *testing.T) {
	_, err := chatToMessages([]byte(`{"model":"claude-3","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1"}]}]}`))
	if err == nil {
		t.Fatal("expected unsupported message field error")
	}
}

func TestResponseToMessagesRejectsUnknownTopLevelFields(t *testing.T) {
	_, err := responseToMessages([]byte(`{"model":"claude-3","input":"hello","include":["usage"]}`))
	if err == nil {
		t.Fatal("expected unsupported unknown Responses field error")
	}
}

func TestResponseToMessagesRejectsUnknownNestedInputFields(t *testing.T) {
	tests := []string{
		`{"model":"claude-3","input":[{"id":"msg_1","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`,
		`{"model":"claude-3","input":[{"role":"user","content":[{"type":"input_text","text":"hello","annotations":[]}]}]}`,
	}
	for _, body := range tests {
		_, err := responseToMessages([]byte(body))
		if err == nil {
			t.Fatalf("expected unsupported nested Responses input field error for %s", body)
		}
	}
}

func TestResponseToMessagesPreservesInputRoles(t *testing.T) {
	body, err := responseToMessages([]byte(`{"model":"claude-3","input":[{"role":"system","content":[{"type":"input_text","text":"be precise"}]},{"role":"assistant","content":[{"type":"input_text","text":"previous"}]},{"role":"user","content":[{"type":"input_text","text":"next"}]}]}`))
	if err != nil {
		t.Fatalf("responseToMessages failed: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"system":"be precise"`, `"role":"assistant"`, `"text":"previous"`, `"role":"user"`, `"text":"next"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %s in %s", want, text)
		}
	}
}

func TestNormalizeResponseOutputsResponsesShape(t *testing.T) {
	body, err := normalizeResponse("claude-3", []byte(`{"content":[{"type":"text","text":"done"}]}`))
	if err != nil {
		t.Fatalf("normalizeResponse failed: %v", err)
	}
	if !strings.Contains(string(body), `"object":"response"`) || !strings.Contains(string(body), `"output_text":"done"`) {
		t.Fatalf("unexpected normalized response: %s", body)
	}
}

func TestNormalizeResponseRejectsMissingText(t *testing.T) {
	if _, err := normalizeResponse("claude-3", []byte(`{"content":[{"type":"image","source":{}}]}`)); err == nil {
		t.Fatal("expected missing text response to fail normalization")
	}
	if _, err := normalizeChat("claude-3", []byte(`{"content":[]}`)); err == nil {
		t.Fatal("expected missing chat text to fail normalization")
	}
}
