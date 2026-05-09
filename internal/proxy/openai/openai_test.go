package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"

	"github.com/gin-gonic/gin"
)

func TestProxy_SessionIDHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := newOpenAITestServer()
	defer server.Close()

	proxy := NewWithResolver(sanitizer.New(detector.New()), nil, server.URL, 5)

	tests := []struct {
		name     string
		path     string
		body     string
		register func(*gin.Engine, *Proxy)
	}{
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt","messages":[{"role":"user","content":"hello"}]}`,
			register: func(router *gin.Engine, p *Proxy) {
				router.POST("/v1/chat/completions", p.HandleChatCompletion)
			},
		},
		{
			name: "responses",
			path: "/v1/responses",
			body: `{"model":"gpt","input":"hello"}`,
			register: func(router *gin.Engine, p *Proxy) {
				router.POST("/v1/responses", p.HandleResponse)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			tt.register(router, proxy)

			t.Run("replaces_invalid_session_id", func(t *testing.T) {
				rec := performRequest(router, tt.path, tt.body, map[string]string{
					"Content-Type":     "application/json",
					"X-Eko-Session-ID": "invalid-session-id",
				})

				if rec.Code != http.StatusOK {
					t.Fatalf("expected status 200, got %d", rec.Code)
				}

				sessionID := rec.Header().Get("X-Eko-Session-ID")
				if sessionID == "" {
					t.Fatal("expected session id header to be set")
				}
				if sessionID == "invalid-session-id" {
					t.Fatal("expected session id header to be replaced")
				}
				if err := tokenizer.ValidateSessionID(sessionID); err != nil {
					t.Fatalf("expected valid session id, got %q", sessionID)
				}
			})

			t.Run("uses_existing_session_id", func(t *testing.T) {
				sessionID := tokenizer.GenerateSessionID()
				rec := performRequest(router, tt.path, tt.body, map[string]string{
					"Content-Type":     "application/json",
					"X-Eko-Session-ID": sessionID,
				})

				if rec.Code != http.StatusOK {
					t.Fatalf("expected status 200, got %d", rec.Code)
				}

				if got := rec.Header().Get("X-Eko-Session-ID"); got != sessionID {
					t.Fatalf("expected session header %q, got %q", sessionID, got)
				}
			})

			t.Run("generates_session_id", func(t *testing.T) {
				rec := performRequest(router, tt.path, tt.body, map[string]string{
					"Content-Type": "application/json",
				})

				if rec.Code != http.StatusOK {
					t.Fatalf("expected status 200, got %d", rec.Code)
				}

				sessionID := rec.Header().Get("X-Eko-Session-ID")
				if sessionID == "" {
					t.Fatal("expected session id header to be set")
				}
				if err := tokenizer.ValidateSessionID(sessionID); err != nil {
					t.Fatalf("expected valid session id, got %q", sessionID)
				}
			})
		})
	}
}

func TestProxy_EnsureSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy := &Proxy{}

	tests := []struct {
		name        string
		sessionID   string
		expectSame  bool
		expectValid bool
	}{
		{
			name:        "generates_when_missing",
			sessionID:   "",
			expectSame:  false,
			expectValid: true,
		},
		{
			name:        "keeps_when_valid",
			sessionID:   tokenizer.GenerateSessionID(),
			expectSame:  true,
			expectValid: true,
		},
		{
			name:        "replaces_when_invalid",
			sessionID:   "invalid-session-id",
			expectSame:  false,
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.sessionID != "" {
				req.Header.Set("X-Eko-Session-ID", tt.sessionID)
			}
			c.Request = req

			got := proxy.ensureSessionID(c)
			respHeader := rec.Header().Get("X-Eko-Session-ID")
			if got == "" {
				t.Fatal("expected session id to be set")
			}
			if respHeader == "" {
				t.Fatal("expected response header to be set")
			}
			if got != respHeader {
				t.Fatalf("expected response header %q to match session id %q", respHeader, got)
			}
			if tt.expectSame && got != tt.sessionID {
				t.Fatalf("expected session id to be preserved, got %q", got)
			}
			if !tt.expectSame && tt.sessionID != "" && got == tt.sessionID {
				t.Fatalf("expected session id to change from %q", tt.sessionID)
			}
			if tt.expectValid {
				if err := tokenizer.ValidateSessionID(got); err != nil {
					t.Fatalf("expected valid session id, got %q", got)
				}
			}
		})
	}
}

func TestProxy_TokenizeMode_NonStreamingChatCompletionAddsHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	capture := &requestCapture{}
	server := newOpenAITestServerWithCapture(capture, false)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	proxy := NewWithResolver(san, nil, server.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	rec := performRequest(router, "/v1/chat/completions", `{"model":"gpt","messages":[{"role":"user","content":"email john@acme.com"}]}`, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got := rec.Header().Get("X-Eko-Sanitization-Mode"); got != "tokenize" {
		t.Fatalf("expected sanitization mode tokenize, got %q", got)
	}
	if got := rec.Header().Get("X-Eko-Sanitization-Override"); got != "" {
		t.Fatalf("expected no sanitization override for non-streaming, got %q", got)
	}
	if got := rec.Header().Get("X-Eko-Tokens-Issued"); got != "1" {
		t.Fatalf("expected tokens issued header 1, got %q", got)
	}
	if got := rec.Header().Get("X-Eko-Session-ID"); got == "" {
		t.Fatal("expected session id header to be set")
	}

	capturedBody := capture.Body()
	if strings.Contains(capturedBody, "john@acme.com") {
		t.Fatalf("expected tokenized request body to not contain original email: %s", capturedBody)
	}
	if strings.Contains(capturedBody, "[REDACTED") {
		t.Fatalf("expected tokenized request body to avoid redaction labels: %s", capturedBody)
	}
}

func TestProxy_TokenizeMode_StreamingChatCompletionUsesRedaction(t *testing.T) {
	gin.SetMode(gin.TestMode)

	capture := &requestCapture{}
	server := newOpenAITestServerWithCapture(capture, true)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	proxy := NewWithResolver(san, nil, server.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	rec := performRequest(router, "/v1/chat/completions", `{"model":"gpt","stream":true,"messages":[{"role":"user","content":"email john@acme.com"}]}`, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got := rec.Header().Get("X-Eko-Sanitization-Mode"); got != "redact" {
		t.Fatalf("expected sanitization mode redact for streaming, got %q", got)
	}
	if got := rec.Header().Get("X-Eko-Sanitization-Override"); got != streamingSanitizationOverride {
		t.Fatalf("expected sanitization override %q for streaming, got %q", streamingSanitizationOverride, got)
	}
	if got := rec.Header().Get("X-Eko-Tokens-Issued"); got != "" {
		t.Fatalf("expected no tokens issued header for streaming, got %q", got)
	}
	if got := rec.Header().Get("X-Eko-Resolve-Status"); got != "" {
		t.Fatalf("expected no resolve status header for streaming, got %q", got)
	}

	capturedBody := capture.Body()
	if !strings.Contains(capturedBody, "[REDACTED_EMAIL]") {
		t.Fatalf("expected redacted email in streaming request body: %s", capturedBody)
	}
}

func TestProxy_TokenizeMode_NonStreamingChatCompletionResolvesTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokenCapture := &tokenCapture{}
	server := newOpenAITestServerWithTokenEcho(tokenCapture, 0)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	resolver := tokenizer.NewResolver(vm)
	proxy := NewWithResolver(san, resolver, server.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	input := `{"model":"gpt","messages":[{"role":"user","content":"email john@acme.com"}]}`
	rec := performRequest(router, "/v1/chat/completions", input, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got := rec.Header().Get("X-Eko-Resolve-Status"); got != "success" {
		t.Fatalf("expected resolve status success, got %q", got)
	}

	token := tokenCapture.Value()
	if token == "" {
		t.Fatal("expected token capture to be set")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "john@acme.com") {
		t.Fatalf("expected response to contain resolved email, got %s", body)
	}
	if strings.Contains(body, token) {
		t.Fatalf("expected response to not contain token, got %s", body)
	}
}

func TestProxy_TokenizeMode_NonStreamingChatCompletionMissingVault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := newOpenAITestServerWithStaticResponse(`{"choices":[{"message":{"role":"assistant","content":"token_123"}}]}`)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	resolver := tokenizer.NewResolver(vm)
	proxy := NewWithResolver(san, resolver, server.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	input := `{"model":"gpt","messages":[{"role":"user","content":"hello world"}]}`
	rec := performRequest(router, "/v1/chat/completions", input, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got := rec.Header().Get("X-Eko-Resolve-Status"); got != "no_vault" {
		t.Fatalf("expected resolve status no_vault, got %q", got)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "token_123") {
		t.Fatalf("expected response to remain unresolved, got %s", body)
	}
}

func TestProxy_TokenizeMode_NonStreamingChatCompletionExpiredVault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokenCapture := &tokenCapture{}
	server := newOpenAITestServerWithTokenEcho(tokenCapture, 10*time.Millisecond)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(1 * time.Millisecond)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	resolver := tokenizer.NewResolver(vm)
	proxy := NewWithResolver(san, resolver, server.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	input := `{"model":"gpt","messages":[{"role":"user","content":"email john@acme.com"}]}`
	rec := performRequest(router, "/v1/chat/completions", input, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusGone {
		t.Fatalf("expected status 410, got %d", rec.Code)
	}

	if got := rec.Header().Get("X-Eko-Resolve-Status"); got != "vault_expired" && got != "no_vault" {
		t.Fatalf("expected resolve status vault_expired or no_vault, got %q", got)
	}

	token := tokenCapture.Value()
	if token == "" {
		t.Fatal("expected token capture to be set")
	}

	if strings.Contains(rec.Body.String(), token) {
		t.Fatalf("expected fail-closed response to avoid unresolved token leakage, got %s", rec.Body.String())
	}
}

func TestProxy_TokenizeMode_NonStreamingResponsesResolvesTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokenCapture := &tokenCapture{}
	server := newOpenAITestServerWithResponseTokenEcho(tokenCapture, 0)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	resolver := tokenizer.NewResolver(vm)
	proxy := NewWithResolver(san, resolver, server.URL, 5)

	router := gin.New()
	router.POST("/v1/responses", proxy.HandleResponse)

	input := `{"model":"gpt","input":"email john@acme.com"}`
	rec := performRequest(router, "/v1/responses", input, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got := rec.Header().Get("X-Eko-Resolve-Status"); got != "success" {
		t.Fatalf("expected resolve status success, got %q", got)
	}

	token := tokenCapture.Value()
	if token == "" {
		t.Fatal("expected token capture to be set")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "john@acme.com") {
		t.Fatalf("expected response to contain resolved email, got %s", body)
	}
	if strings.Contains(body, token) {
		t.Fatalf("expected response to not contain token, got %s", body)
	}
}

func TestProxy_TokenizeMode_NonStreamingResponsesSanitizeStoreFailureReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := newOpenAITestServer()
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	tok := tokenizer.NewTokenizer()
	store := &failingSessionStore{beginErr: tokenizer.ErrSessionStoreUnavailable}
	san := sanitizer.NewWithTokenizer(det, tok, store, "tokenize")
	proxy := NewWithResolver(san, nil, server.URL, 5)

	router := gin.New()
	router.POST("/v1/responses", proxy.HandleResponse)

	rec := performRequest(router, "/v1/responses", `{"model":"gpt","input":"email john@acme.com"}`, map[string]string{
		"Content-Type": "application/json",
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestProxy_TokenizeMode_ReusesExistingSessionForResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := newOpenAITestServerWithStaticResponse(`{"choices":[{"message":{"role":"assistant","content":"masked@example.com"}}]}`)
	defer server.Close()

	det := detector.New()
	loadTestPatterns(t, det)
	vm := tokenizer.NewVaultManager(5 * time.Minute)
	tok := tokenizer.NewTokenizer()
	sessionID := tokenizer.GenerateSessionID()
	vault, err := vm.GetOrCreate(sessionID)
	if err != nil {
		t.Fatalf("get or create vault: %v", err)
	}
	if err := vault.Store("john@acme.com", "masked@example.com"); err != nil {
		t.Fatalf("store token: %v", err)
	}

	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")
	resolver := tokenizer.NewResolver(vm)
	proxy := NewWithResolver(san, resolver, server.URL, 5)

	router := gin.New()
	router.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	rec := performRequest(router, "/v1/chat/completions", `{"model":"gpt","messages":[{"role":"user","content":"hello world"}]}`, map[string]string{
		"Content-Type":     "application/json",
		"X-Eko-Session-ID": sessionID,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Eko-Resolve-Status"); got != "success" {
		t.Fatalf("expected resolve status success, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "john@acme.com") {
		t.Fatalf("expected prior session token to be resolved, got %s", rec.Body.String())
	}
}

func performRequest(router *gin.Engine, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

type requestCapture struct {
	mu   sync.Mutex
	body string
}

func (c *requestCapture) Set(body string) {
	c.mu.Lock()
	c.body = body
	c.mu.Unlock()
}

func (c *requestCapture) Body() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

type tokenCapture struct {
	mu    sync.Mutex
	token string
}

func (c *tokenCapture) Set(token string) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

func (c *tokenCapture) Value() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

func newOpenAITestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test"}`))
	})
	mux.HandleFunc("/responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"response_test"}`))
	})
	return httptest.NewServer(mux)
}

func loadTestPatterns(t *testing.T, det *detector.Detector) {
	t.Helper()

	defaultDir := filepath.Join("..", "..", "..", "patterns", "default")
	customDir := filepath.Join("..", "..", "..", "patterns", "custom")
	loader := patterns.NewLoader(defaultDir, customDir)
	compiled, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}
	det.LoadPatterns(compiled)
}

func newOpenAITestServerWithCapture(capture *requestCapture, streaming bool) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.Set(string(body))
		if streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test"}`))
	})
	return httptest.NewServer(mux)
}

func newOpenAITestServerWithTokenEcho(capture *tokenCapture, delay time.Duration) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		token := ""
		if len(req.Messages) > 0 {
			token = req.Messages[0].Content
		}
		capture.Set(token)
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello ` + token + `"}}]}`))
	})
	return httptest.NewServer(mux)
}

func newOpenAITestServerWithResponseTokenEcho(capture *tokenCapture, delay time.Duration) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/responses", func(w http.ResponseWriter, r *http.Request) {
		var req CreateResponseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input string
		_ = json.Unmarshal(req.Input, &input)
		capture.Set(input)
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"output_text":"hello ` + input + `"}`))
	})
	return httptest.NewServer(mux)
}

func newOpenAITestServerWithStaticResponse(response string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	})
	return httptest.NewServer(mux)
}

type failingSessionStore struct {
	beginErr error
	getErr   error
}

func (s *failingSessionStore) BeginSession(context.Context, string) (tokenizer.SessionHandle, error) {
	return nil, s.beginErr
}

func (s *failingSessionStore) GetSession(context.Context, string) (*tokenizer.Vault, error) {
	if s.getErr == nil {
		return nil, tokenizer.ErrVaultNotFound
	}
	return nil, s.getErr
}

func (s *failingSessionStore) HasSession(context.Context, string) (bool, error) {
	if s.getErr == nil {
		return false, nil
	}
	return false, s.getErr
}

func (s *failingSessionStore) DeleteSession(context.Context, string) error {
	return nil
}

func (s *failingSessionStore) Close() error {
	return nil
}
