package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestCopyProviderHeadersGeminiEncodesAndPreservesQuery(t *testing.T) {
	upstream, err := http.NewRequest(http.MethodPost, "https://gemini.test/v1/models/x:generateContent?alt=json", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req := RouteRequest{
		Query: map[string][]string{
			"key": {"abc+/= 123"},
		},
		Headers: http.Header{},
	}

	CopyProviderHeaders(req, upstream, ProviderGemini)

	query := upstream.URL.Query()
	if got := query.Get("alt"); got != "json" {
		t.Fatalf("alt query = %q, want json", got)
	}
	if got := query.Get("key"); got != "abc+/= 123" {
		t.Fatalf("key query = %q, want original value", got)
	}
	if upstream.URL.RawQuery == "key=abc+/= 123" {
		t.Fatalf("expected encoded raw query, got %q", upstream.URL.RawQuery)
	}
}

func TestResponsesInputTextMessagesRejectsEmptyArrays(t *testing.T) {
	tests := []string{
		`[]`,
		`[{"role":"user","content":[]}]`,
	}
	for _, body := range tests {
		_, err := ResponsesInputTextMessages(json.RawMessage(body), ProviderAnthropic, "claude-3")
		if err == nil {
			t.Fatalf("expected empty Responses input to be rejected for %s", body)
		}
	}
}

func TestResponsesInputTextMessagesPreservesRolesAndText(t *testing.T) {
	got, err := ResponsesInputTextMessages(json.RawMessage(`[{"role":"system","content":[{"type":"input_text","text":"rules"}]},{"role":"assistant","content":[{"type":"input_text","text":"previous"}]},{"role":"user","content":[{"type":"input_text","text":"next"}]}]`), ProviderGemini, "gemini-pro")
	if err != nil {
		t.Fatalf("ResponsesInputTextMessages failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("message count = %d, want 3", len(got))
	}
	for i, want := range []TextMessage{{Role: "system", Content: "rules"}, {Role: "assistant", Content: "previous"}, {Role: "user", Content: "next"}} {
		if got[i] != want {
			t.Fatalf("message[%d] = %+v, want %+v", i, got[i], want)
		}
	}
}

func TestProxyFailureClassifiesTimeout(t *testing.T) {
	err := ProxyFailure(ProviderOpenAI, "/v1/chat/completions", "gpt-4o", context.DeadlineExceeded)
	if err.Status != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", err.Status)
	}
	if err.Type != "proxy_timeout" {
		t.Fatalf("type = %q, want proxy_timeout", err.Type)
	}
}

func TestProxyFailureKeepsNonTimeoutAsBadGateway(t *testing.T) {
	err := ProxyFailure(ProviderOpenAI, "/v1/chat/completions", "gpt-4o", errors.New("connection refused"))
	if err.Status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", err.Status)
	}
	if err.Type != "proxy_error" {
		t.Fatalf("type = %q, want proxy_error", err.Type)
	}
}
