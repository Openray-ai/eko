package test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"
	"eko/internal/proxy/openai"

	"github.com/gin-gonic/gin"
)

func TestOpenAIProxy_TokenizationResolutionEndToEnd(t *testing.T) {
	if os.Getenv("EKO_INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set EKO_INTEGRATION_TEST=true to run")
	}

	gin.SetMode(gin.TestMode)

	openaiServer := newTokenEchoServer(t)
	defer openaiServer.Close()

	det := detector.New()
	loadIntegrationPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	resolver := tokenizer.NewResolver(vm)
	proxy := openai.NewWithResolver(san, resolver, openaiServer.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	payload := map[string]any{
		"model": "gpt",
		"messages": []map[string]string{
			{"role": "user", "content": "email john@acme.com"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", proxyServer.URL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	resolvedBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("X-Eko-Resolve-Status"); got != "success" {
		t.Fatalf("expected resolve status success, got %q", got)
	}

	if !bytes.Contains(resolvedBody, []byte("john@acme.com")) {
		t.Fatalf("expected resolved response to contain email, got %s", string(resolvedBody))
	}
}

func newTokenEchoServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		token := ""
		if len(req.Messages) > 0 {
			token = req.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello ` + token + `"}}]}`))
	})

	return httptest.NewServer(mux)
}

func loadIntegrationPatterns(t *testing.T, det *detector.Detector) {
	t.Helper()

	defaultDir := filepath.Join("..", "patterns", "default")
	customDir := filepath.Join("..", "patterns", "custom")
	loader := patterns.NewLoader(defaultDir, customDir)
	compiled, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}
	det.LoadPatterns(compiled)
}
