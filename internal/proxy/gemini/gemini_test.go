package gemini

import (
	"strings"
	"testing"
)

func TestChatToGenerateContentMapsSystemAndMessages(t *testing.T) {
	body, err := chatToGenerateContent([]byte(`{"model":"gemini-pro","messages":[{"role":"system","content":"be precise"},{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("chatToGenerateContent failed: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"systemInstruction"`, `"role":"user"`, `"role":"model"`, `"text":"hello"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %s in %s", want, text)
		}
	}
}

func TestChatToGenerateContentRejectsUnsupportedTools(t *testing.T) {
	_, err := chatToGenerateContent([]byte(`{"model":"gemini-pro","tools":[{"type":"function"}],"messages":[{"role":"user","content":"hello"}]}`))
	if err == nil {
		t.Fatal("expected unsupported tools error")
	}
}

func TestChatToGenerateContentRejectsUnknownTopLevelFields(t *testing.T) {
	_, err := chatToGenerateContent([]byte(`{"model":"gemini-pro","presence_penalty":1,"messages":[{"role":"user","content":"hello"}]}`))
	if err == nil {
		t.Fatal("expected unsupported unknown field error")
	}
}

func TestChatToGenerateContentRejectsUnsupportedMessageFields(t *testing.T) {
	_, err := chatToGenerateContent([]byte(`{"model":"gemini-pro","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1"}]}]}`))
	if err == nil {
		t.Fatal("expected unsupported message field error")
	}
}

func TestResponseToGenerateContentRejectsUnknownTopLevelFields(t *testing.T) {
	_, err := responseToGenerateContent([]byte(`{"model":"gemini-pro","input":"hello","include":["usage"]}`))
	if err == nil {
		t.Fatal("expected unsupported unknown Responses field error")
	}
}

func TestResponseToGenerateContentRejectsUnknownNestedInputFields(t *testing.T) {
	tests := []string{
		`{"model":"gemini-pro","input":[{"id":"msg_1","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`,
		`{"model":"gemini-pro","input":[{"role":"user","content":[{"type":"input_text","text":"hello","annotations":[]}]}]}`,
	}
	for _, body := range tests {
		_, err := responseToGenerateContent([]byte(body))
		if err == nil {
			t.Fatalf("expected unsupported nested Responses input field error for %s", body)
		}
	}
}

func TestResponseToGenerateContentPreservesInputRoles(t *testing.T) {
	body, err := responseToGenerateContent([]byte(`{"model":"gemini-pro","input":[{"role":"system","content":[{"type":"input_text","text":"be precise"}]},{"role":"assistant","content":[{"type":"input_text","text":"previous"}]},{"role":"user","content":[{"type":"input_text","text":"next"}]}]}`))
	if err != nil {
		t.Fatalf("responseToGenerateContent failed: %v", err)
	}
	text := string(body)
	for _, want := range []string{`"systemInstruction"`, `"text":"be precise"`, `"role":"model"`, `"text":"previous"`, `"role":"user"`, `"text":"next"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %s in %s", want, text)
		}
	}
}

func TestGenerateContentPathEscapesModelSegment(t *testing.T) {
	got := generateContentPath("publishers/google/models/gemini-pro")
	want := "/models/publishers%2Fgoogle%2Fmodels%2Fgemini-pro:generateContent"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestNormalizeResponseOutputsResponsesShape(t *testing.T) {
	body, err := normalizeResponse("gemini-pro", []byte(`{"candidates":[{"content":{"parts":[{"text":"done"}]}}]}`))
	if err != nil {
		t.Fatalf("normalizeResponse failed: %v", err)
	}
	if !strings.Contains(string(body), `"object":"response"`) || !strings.Contains(string(body), `"output_text":"done"`) {
		t.Fatalf("unexpected normalized response: %s", body)
	}
}

func TestNormalizeResponseRejectsMissingText(t *testing.T) {
	if _, err := normalizeResponse("gemini-pro", []byte(`{"candidates":[{"content":{"parts":[{"inlineData":{}}]}}]}`)); err == nil {
		t.Fatal("expected missing text response to fail normalization")
	}
	if _, err := normalizeChat("gemini-pro", []byte(`{"candidates":[]}`)); err == nil {
		t.Fatal("expected missing chat text to fail normalization")
	}
}
