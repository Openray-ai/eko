package middleware

import (
	"net/url"
	"strings"
	"testing"
)

func TestRedactedRawQueryRedactsSensitiveValues(t *testing.T) {
	query := url.Values{
		"key":           {"gemini-secret"},
		"api_key":       {"api-secret"},
		"access_token":  {"token-secret"},
		"Authorization": {"bearer-secret"},
		"alt":           {"json"},
	}

	got := redactedRawQuery(query)

	for _, leaked := range []string{"gemini-secret", "api-secret", "token-secret", "bearer-secret"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("query leaked sensitive value %q in %q", leaked, got)
		}
	}
	if !strings.Contains(got, "alt=json") {
		t.Fatalf("expected non-sensitive query parameter to be preserved, got %q", got)
	}
	if strings.Count(got, "%5BREDACTED%5D") != 4 {
		t.Fatalf("expected four redacted values, got %q", got)
	}
}

func TestRedactedRawQueryEmpty(t *testing.T) {
	if got := redactedRawQuery(nil); got != "" {
		t.Fatalf("empty query = %q, want empty string", got)
	}
}
