package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/sanitizer"
	"eko/internal/proxy/anthropic"
	"eko/internal/proxy/common"
	"eko/internal/proxy/deepseek"
	"eko/internal/proxy/gemini"
	"eko/internal/proxy/openai"

	"github.com/gin-gonic/gin"
)

func TestModelRouterResolvePrecedence(t *testing.T) {
	router := NewModelRouter(RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Models: map[string]common.ProviderName{
			"claude-special": common.ProviderDeepSeek,
		},
		Prefixes: map[string]common.ProviderName{
			"claude-": common.ProviderAnthropic,
			"gemini-": common.ProviderGemini,
		},
	})

	tests := []struct {
		model string
		want  common.ProviderName
	}{
		{"claude-special", common.ProviderDeepSeek},
		{"claude-3-5-sonnet", common.ProviderAnthropic},
		{"gemini-1.5-pro", common.ProviderGemini},
		{"unmapped-model", common.ProviderOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, err := router.Resolve("/v1/chat/completions", tt.model)
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}
			if got.Provider != tt.want {
				t.Fatalf("provider = %q, want %q", got.Provider, tt.want)
			}
		})
	}
}

func TestProxyRoutesChatCompletionsByModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openaiCapture, openaiServer := newCaptureServer(t, "/chat/completions", `{"choices":[{"message":{"content":"openai"}}]}`)
	defer openaiServer.Close()
	anthropicCapture, anthropicServer := newCaptureServer(t, "/messages", `{"content":[{"type":"text","text":"anthropic"}]}`)
	defer anthropicServer.Close()
	geminiCapture, geminiServer := newCaptureServer(t, "/models/gemini-pro:generateContent", `{"candidates":[{"content":{"parts":[{"text":"gemini"}]}}]}`)
	defer geminiServer.Close()
	deepseekCapture, deepseekServer := newCaptureServer(t, "/chat/completions", `{"choices":[{"message":{"content":"deepseek"}}]}`)
	defer deepseekServer.Close()

	proxy := New(sanitizer.New(loadDetector(t)), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"gpt-":      common.ProviderOpenAI,
			"claude-":   common.ProviderAnthropic,
			"gemini-":   common.ProviderGemini,
			"deepseek-": common.ProviderDeepSeek,
		},
	}, []common.Adapter{
		openai.NewAdapter(openaiServer.URL, 5),
		anthropic.New(anthropicServer.URL, 5),
		gemini.New(geminiServer.URL, 5),
		deepseek.New(deepseekServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	tests := []struct {
		name         string
		model        string
		bodyContains string
		capture      *capture
	}{
		{"openai", "gpt-4o", `"model":"gpt-4o"`, openaiCapture},
		{"anthropic", "claude-3-5-sonnet", `"max_tokens":1024`, anthropicCapture},
		{"gemini", "gemini-pro", `"systemInstruction"`, geminiCapture},
		{"deepseek", "deepseek-chat", `"reasoning_effort":"high"`, deepseekCapture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"model":"` + tt.model + `","reasoning_effort":"high","messages":[{"role":"system","content":"be precise"},{"role":"user","content":"hello"}]}`
			if tt.name == "anthropic" || tt.name == "gemini" {
				body = `{"model":"` + tt.model + `","messages":[{"role":"system","content":"be precise"},{"role":"user","content":"hello"}]}`
			}
			rec := performRequest(engine, "/v1/chat/completions", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(tt.capture.Body(), tt.bodyContains) {
				t.Fatalf("expected upstream body to contain %s, got %s", tt.bodyContains, tt.capture.Body())
			}
		})
	}
}

func TestProxyPreservesOpenAICompatibleChatMessageFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openaiCapture, openaiServer := newCaptureServer(t, "/chat/completions", `{"choices":[{"message":{"content":"openai"}}]}`)
	defer openaiServer.Close()

	proxy := New(sanitizer.New(loadDetector(t)), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"gpt-": common.ProviderOpenAI,
		},
	}, []common.Adapter{
		openai.NewAdapter(openaiServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	body := `{
		"model":"gpt-4o",
		"messages":[
			{"role":"user","name":"alice","content":"email secret@example.com"},
			{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"email\":\"tool@example.com\"}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"done"}
		]
	}`
	rec := performRequest(engine, "/v1/chat/completions", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	upstream := openaiCapture.Body()
	for _, want := range []string{`"name":"alice"`, `"tool_calls":[`, `"tool_call_id":"call_1"`, `"content":null`} {
		if !strings.Contains(upstream, want) {
			t.Fatalf("expected upstream body to preserve %s, got %s", want, upstream)
		}
	}
	for _, leaked := range []string{"secret@example.com", "tool@example.com"} {
		if strings.Contains(upstream, leaked) {
			t.Fatalf("expected chat text fields to be sanitized, got %s", upstream)
		}
	}
}

func TestProxyRoutesResponsesByModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	anthropicCapture, anthropicServer := newCaptureServer(t, "/messages", `{"content":[{"type":"text","text":"anthropic response"}]}`)
	defer anthropicServer.Close()
	geminiCapture, geminiServer := newCaptureServer(t, "/models/gemini-pro:generateContent", `{"candidates":[{"content":{"parts":[{"text":"gemini response"}]}}]}`)
	defer geminiServer.Close()
	deepseekCapture, deepseekServer := newCaptureServer(t, "/chat/completions", `{"choices":[{"message":{"content":"deepseek response"}}]}`)
	defer deepseekServer.Close()

	proxy := New(sanitizer.New(loadDetector(t)), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"claude-":   common.ProviderAnthropic,
			"gemini-":   common.ProviderGemini,
			"deepseek-": common.ProviderDeepSeek,
		},
	}, []common.Adapter{
		anthropic.New(anthropicServer.URL, 5),
		gemini.New(geminiServer.URL, 5),
		deepseek.New(deepseekServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/responses", proxy.HandleResponse)

	tests := []struct {
		model   string
		want    string
		capture *capture
	}{
		{"claude-3-5-sonnet", "anthropic response", anthropicCapture},
		{"gemini-pro", "gemini response", geminiCapture},
		{"deepseek-chat", "deepseek response", deepseekCapture},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			rec := performRequest(engine, "/v1/responses", `{"model":"`+tt.model+`","input":"hello"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("expected normalized response to contain %q, got %s", tt.want, rec.Body.String())
			}
			if !strings.Contains(tt.capture.Body(), "hello") {
				t.Fatalf("expected upstream request to contain input text, got %s", tt.capture.Body())
			}
		})
	}
}

func TestProxyRejectsUnsupportedFieldsAndUnsafeStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy := New(sanitizer.New(detector.New()), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"claude-": common.ProviderAnthropic,
			"gemini-": common.ProviderGemini,
		},
	}, []common.Adapter{
		anthropic.New("http://127.0.0.1", 5),
		gemini.New("http://127.0.0.1", 5),
	})

	engine := gin.New()
	engine.POST("/v1/chat/completions", proxy.HandleChatCompletion)
	engine.POST("/v1/responses", proxy.HandleResponse)

	chat := performRequest(engine, "/v1/chat/completions", `{"model":"claude-3","tools":[{"type":"function"}],"messages":[{"role":"user","content":"hello"}]}`)
	if chat.Code != http.StatusBadRequest {
		t.Fatalf("expected chat unsupported field status 400, got %d", chat.Code)
	}

	responses := performRequest(engine, "/v1/responses", `{"model":"gemini-pro","input":[{"role":"user","content":[{"type":"input_image","image_url":"x"}]}]}`)
	if responses.Code != http.StatusBadRequest {
		t.Fatalf("expected responses unsupported field status 400, got %d", responses.Code)
	}

	stream := performRequest(engine, "/v1/chat/completions", `{"model":"claude-3","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	if stream.Code != http.StatusBadRequest {
		t.Fatalf("expected streaming unsupported status 400, got %d", stream.Code)
	}
}

func TestProxyPreservesOpenAIResponsesNonTextBlocks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openaiCapture, openaiServer := newCaptureServer(t, "/responses", `{"id":"response_test"}`)
	defer openaiServer.Close()

	proxy := New(sanitizer.New(loadDetector(t)), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"gpt-": common.ProviderOpenAI,
		},
	}, []common.Adapter{
		openai.NewAdapter(openaiServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/responses", proxy.HandleResponse)

	rec := performRequest(engine, "/v1/responses", `{"model":"gpt-4.1","input":[{"role":"user","id":"msg_1","content":[{"type":"input_text","text":"email secret@example.com","annotations":[{"type":"note","text":"keep"}]},{"type":"input_image","image_url":"https://example.com/image.jpg","detail":"high"},{"type":"input_file","file_id":"file_123"}]}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	upstream := openaiCapture.Body()
	for _, want := range []string{`"id":"msg_1"`, `"annotations":[`, `"type":"input_image"`, `"image_url":"https://example.com/image.jpg"`, `"detail":"high"`, `"type":"input_file"`, `"file_id":"file_123"`} {
		if !strings.Contains(upstream, want) {
			t.Fatalf("expected OpenAI upstream body to preserve %s, got %s", want, upstream)
		}
	}
	if strings.Contains(upstream, "secret@example.com") {
		t.Fatalf("expected input_text to be sanitized, got %s", upstream)
	}
}

func TestProxyRejectsNonOpenAIResponsesInstructions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy := New(sanitizer.New(detector.New()), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"claude-":   common.ProviderAnthropic,
			"gemini-":   common.ProviderGemini,
			"deepseek-": common.ProviderDeepSeek,
		},
	}, []common.Adapter{
		anthropic.New("http://127.0.0.1", 5),
		gemini.New("http://127.0.0.1", 5),
		deepseek.New("http://127.0.0.1", 5),
	})

	engine := gin.New()
	engine.POST("/v1/responses", proxy.HandleResponse)

	for _, model := range []string{"claude-3", "gemini-pro", "deepseek-chat"} {
		t.Run(model, func(t *testing.T) {
			rec := performRequest(engine, "/v1/responses", `{"model":"`+model+`","instructions":"do not ignore this","input":"hello"}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "responses_advanced_fields") {
				t.Fatalf("expected advanced fields capability error, got %s", rec.Body.String())
			}
		})
	}
}

func TestProxyEnforcesAdapterCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxy := New(sanitizer.New(detector.New()), nil, RoutingConfig{
		DefaultProvider: common.ProviderName("limited"),
	}, []common.Adapter{
		limitedAdapter{},
	})

	engine := gin.New()
	engine.POST("/v1/responses", proxy.HandleResponse)

	rec := performRequest(engine, "/v1/responses", `{"model":"anything","input":"hello"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"capability":"responses"`) {
		t.Fatalf("expected responses capability error, got %s", rec.Body.String())
	}
}

func TestProxyEnforcesRouteSpecificStreamingCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)

	adapter := &chatStreamingOnlyAdapter{}
	proxy := New(sanitizer.New(detector.New()), nil, RoutingConfig{
		DefaultProvider: common.ProviderName("chat-stream-only"),
	}, []common.Adapter{
		adapter,
	})

	engine := gin.New()
	engine.POST("/v1/responses", proxy.HandleResponse)

	rec := performRequest(engine, "/v1/responses", `{"model":"anything","stream":true,"input":"hello"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if adapter.responsesCalled {
		t.Fatal("expected router to reject Responses streaming before calling adapter")
	}
	if !strings.Contains(rec.Body.String(), `"capability":"responses_streaming"`) {
		t.Fatalf("expected responses_streaming capability error, got %s", rec.Body.String())
	}
}

func TestProxyAddsViolationSummaryHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openaiCapture, openaiServer := newCaptureServer(t, "/chat/completions", `{"choices":[{"message":{"content":"ok"}}]}`)
	defer openaiServer.Close()

	proxy := New(sanitizer.New(loadDetector(t)), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"gpt-": common.ProviderOpenAI,
		},
	}, []common.Adapter{
		openai.NewAdapter(openaiServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	rec := performRequest(engine, "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"email secret@example.com"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Eko-Violation-Summary"); got == "" || !strings.Contains(got, "pii") {
		t.Fatalf("expected violation summary header with pii, got %q", got)
	}
	if strings.Contains(openaiCapture.Body(), "secret@example.com") {
		t.Fatalf("expected upstream body to be sanitized, got %s", openaiCapture.Body())
	}
}

func TestProxyStreamingViolationEventsPreserveLegacyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer openaiServer.Close()

	proxy := New(sanitizer.New(loadDetector(t)), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"gpt-": common.ProviderOpenAI,
		},
	}, []common.Adapter{
		openai.NewAdapter(openaiServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/chat/completions", proxy.HandleChatCompletion)

	rec := performRequest(engine, "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"email secret@example.com"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"type":"eko.violation_report"`, `"type":"eko.violation"`, `"summary":"pii:1"`, `"details"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected stream body to contain %s, got %s", want, body)
		}
	}
	if got := rec.Header().Get("X-Eko-Sanitization-Override"); got != streamingSanitizationOverride {
		t.Fatalf("streaming override = %q, want %q", got, streamingSanitizationOverride)
	}
}

func TestProxyPreservesStreamingUpstreamErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer openaiServer.Close()

	proxy := New(sanitizer.New(detector.New()), nil, RoutingConfig{
		DefaultProvider: common.ProviderOpenAI,
		Prefixes: map[string]common.ProviderName{
			"gpt-": common.ProviderOpenAI,
		},
	}, []common.Adapter{
		openai.NewAdapter(openaiServer.URL, 5),
	})

	engine := gin.New()
	engine.POST("/v1/chat/completions", proxy.HandleChatCompletion)
	engine.POST("/v1/responses", proxy.HandleResponse)

	chat := performRequest(engine, "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	if chat.Code != http.StatusUnauthorized {
		t.Fatalf("expected streamed chat status 401, got %d body=%s", chat.Code, chat.Body.String())
	}
	if got := chat.Header().Get("X-Eko-Sanitization-Override"); got != "" {
		t.Fatalf("expected no streaming override header on upstream error, got %q", got)
	}
	if !strings.Contains(chat.Body.String(), "bad key") {
		t.Fatalf("expected upstream error body, got %s", chat.Body.String())
	}

	responses := performRequest(engine, "/v1/responses", `{"model":"gpt-4o","stream":true,"input":"hello"}`)
	if responses.Code != http.StatusUnauthorized {
		t.Fatalf("expected streamed Responses status 401, got %d body=%s", responses.Code, responses.Body.String())
	}
	if got := responses.Header().Get("X-Eko-Sanitization-Override"); got != "" {
		t.Fatalf("expected no streaming override header on upstream error, got %q", got)
	}
	if !strings.Contains(responses.Body.String(), "bad key") {
		t.Fatalf("expected upstream error body, got %s", responses.Body.String())
	}
}

func performRequest(engine *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

type limitedAdapter struct{}

func (limitedAdapter) Name() common.ProviderName { return common.ProviderName("limited") }
func (limitedAdapter) Capabilities() common.Capabilities {
	return common.Capabilities{ChatCompletions: true}
}
func (limitedAdapter) ChatCompletions(common.RouteRequest) (*common.RouteResponse, error) {
	return nil, errors.New("should not be called")
}
func (limitedAdapter) Responses(common.RouteRequest) (*common.RouteResponse, error) {
	return nil, errors.New("should not be called")
}

type chatStreamingOnlyAdapter struct {
	responsesCalled bool
}

func (*chatStreamingOnlyAdapter) Name() common.ProviderName {
	return common.ProviderName("chat-stream-only")
}
func (*chatStreamingOnlyAdapter) Capabilities() common.Capabilities {
	return common.Capabilities{
		ChatCompletions: true,
		Responses:       true,
		ChatStreaming:   true,
	}
}
func (*chatStreamingOnlyAdapter) ChatCompletions(common.RouteRequest) (*common.RouteResponse, error) {
	return nil, errors.New("should not be called")
}
func (a *chatStreamingOnlyAdapter) Responses(common.RouteRequest) (*common.RouteResponse, error) {
	a.responsesCalled = true
	return nil, errors.New("should not be called")
}

type capture struct {
	mu   sync.Mutex
	body string
}

func (c *capture) Set(body string) {
	c.mu.Lock()
	c.body = body
	c.mu.Unlock()
}

func (c *capture) Body() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

func newCaptureServer(t *testing.T, wantPath, response string) (*capture, *httptest.Server) {
	t.Helper()
	capture := &capture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("unexpected path %s, want %s", r.URL.Path, wantPath)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("invalid upstream json: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		encoded, _ := json.Marshal(raw)
		capture.Set(string(encoded))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	return capture, server
}

func loadDetector(t *testing.T) *detector.Detector {
	t.Helper()
	det := detector.New()
	defaultDir := filepath.Join("..", "..", "..", "patterns", "default")
	customDir := filepath.Join("..", "..", "..", "patterns", "custom")
	loader := patterns.NewLoader(defaultDir, customDir)
	compiled, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}
	det.LoadPatterns(compiled)
	return det
}
