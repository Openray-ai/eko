package main

import (
	"eko/internal/api/handlers"
	"eko/internal/config"
	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/tokenizer"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServerBootsWithTokenizeMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.Default()
	cfg.Proxy.Behavior.SanitizationMode = "tokenize"
	cfg.Proxy.Behavior.TokenTTLms = 1500
	cfg.Proxy.OpenAI.Enabled = true

	det := detector.New()
	loadTestPatterns(t, det)

	san, resolver, store := buildSanitizer(cfg, det)
	defer func() {
		if store != nil {
			_ = store.Close()
		}
	}()
	if resolver == nil {
		t.Fatal("expected resolver to be initialized for tokenize mode")
	}

	openaiProxy := buildOpenAIProxy(cfg, san, resolver)
	if openaiProxy == nil {
		t.Fatal("expected openai proxy to be initialized")
	}
	if openaiProxy.GetResolver() == nil {
		t.Fatal("expected openai proxy to receive resolver")
	}

	metrics := handlers.NewMetricsCollector()
	router := buildRouter(cfg, metrics, san, openaiProxy, store)
	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("failed to call health endpoint: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", resp.StatusCode)
	}

	sessionID := tokenizer.GenerateSessionID()
	input := "Account ACCT1234"
	result, err := san.SanitizeWithSession(input, sessionID)
	if err != nil {
		t.Fatalf("sanitize with session failed: %v", err)
	}
	if result.TokenizedCount == 0 {
		t.Fatalf("expected tokenized count > 0")
	}
	if result.SanitizedPrompt == input {
		t.Fatalf("expected sanitized prompt to change")
	}

	resolved, err := resolver.ResolveResponse([]byte(result.SanitizedPrompt), sessionID)
	if err != nil {
		t.Fatalf("resolve response failed: %v", err)
	}
	if string(resolved) != input {
		t.Fatalf("expected resolved output to match original input")
	}
}

func loadTestPatterns(t *testing.T, det *detector.Detector) {
	t.Helper()

	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "account_id"
    regex: "ACCT\\d{4}"
    type: "pii"
    severity: "BLOCK"
    description: "Account ID"
`

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write pattern file: %v", err)
	}

	loader := patterns.NewLoader(yamlPath, "")
	compiledPatterns, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}

	det.LoadPatterns(compiledPatterns)
}
