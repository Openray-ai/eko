package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/sanitizer"
	"eko/internal/core/slm"
	"eko/internal/core/tokenizer"

	"github.com/gin-gonic/gin"
)

func TestSanitizeHandler_TokenizeWithSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	det := detector.New()
	loadTestPatterns(t, det)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")

	handler := NewSanitizeHandler(san)
	router := gin.New()
	router.POST("/v1/sanitize", handler.Handle)

	reqBody := map[string]string{
		"prompt": "My email is john.doe@example.com",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sanitize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp sanitizer.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.SessionID == "" {
		t.Fatal("expected session_id to be returned")
	}
	if err := tokenizer.ValidateSessionID(resp.SessionID); err != nil {
		t.Fatalf("expected valid session_id, got %q", resp.SessionID)
	}
	if resp.TokenizedCount == 0 {
		t.Fatalf("expected tokenized_count > 0, got %d", resp.TokenizedCount)
	}
	if bytes.Contains([]byte(resp.SanitizedPrompt), []byte("john.doe@example.com")) {
		t.Fatalf("expected sanitized prompt to not contain original email, got %q", resp.SanitizedPrompt)
	}
	if bytes.Contains([]byte(resp.SanitizedPrompt), []byte("[REDACTED")) {
		t.Fatalf("expected tokenized prompt, got redaction label %q", resp.SanitizedPrompt)
	}
}

func TestSanitizeHandler_UsesProvidedSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	det := detector.New()
	loadTestPatterns(t, det)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")

	handler := NewSanitizeHandler(san)
	router := gin.New()
	router.POST("/v1/sanitize", handler.Handle)

	sessionID := tokenizer.GenerateSessionID()
	reqBody := map[string]string{
		"prompt":     "My email is john.doe@example.com",
		"session_id": sessionID,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sanitize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp sanitizer.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.SessionID != sessionID {
		t.Fatalf("expected session_id %q, got %q", sessionID, resp.SessionID)
	}
}

// stubSLM implements detector.SLMRunner for handler tests.
type stubSLM struct {
	called bool
}

func (s *stubSLM) Detect(_ context.Context, _ string) ([]slm.Violation, error) {
	s.called = true
	return nil, nil
}

func TestSanitizeHandler_SLMOptIn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		body        string
		expectCalls bool
	}{
		{"omitted defaults to off", `{"prompt":"hi"}`, false},
		{"explicit false", `{"prompt":"hi","slm":false}`, false},
		{"explicit true", `{"prompt":"hi","slm":true}`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			det := detector.New()
			loadTestPatterns(t, det)
			stub := &stubSLM{}
			det.SetSLM(stub)

			tok := tokenizer.NewTokenizer()
			vm := tokenizer.NewVaultManager(1 * time.Minute)
			san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")

			handler := NewSanitizeHandler(san)
			router := gin.New()
			router.POST("/v1/sanitize", handler.Handle)

			req := httptest.NewRequest(http.MethodPost, "/v1/sanitize", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			if stub.called != tc.expectCalls {
				t.Fatalf("SLM called=%v, want %v", stub.called, tc.expectCalls)
			}
		})
	}
}

func TestSanitizeHandler_SanitizationModeOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name           string
		body           string
		wantStatus     int
		wantTokenized  bool // true => tokenized_count>0; false => 0 (redact path)
		wantPromptSubs string
	}{
		{
			name:          "omitted uses configured tokenize default",
			body:          `{"prompt":"My email is john.doe@example.com"}`,
			wantStatus:    http.StatusOK,
			wantTokenized: true,
		},
		{
			name:           "explicit redact overrides tokenize default",
			body:           `{"prompt":"My email is john.doe@example.com","sanitization_mode":"redact"}`,
			wantStatus:     http.StatusOK,
			wantTokenized:  false,
			wantPromptSubs: "[REDACTED_EMAIL]",
		},
		{
			name:          "explicit tokenize matches configured default",
			body:          `{"prompt":"My email is john.doe@example.com","sanitization_mode":"tokenize"}`,
			wantStatus:    http.StatusOK,
			wantTokenized: true,
		},
		{
			name:       "invalid mode rejected",
			body:       `{"prompt":"x","sanitization_mode":"obfuscate"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			det := detector.New()
			loadTestPatterns(t, det)

			tok := tokenizer.NewTokenizer()
			vm := tokenizer.NewVaultManager(1 * time.Minute)
			san := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")

			handler := NewSanitizeHandler(san)
			router := gin.New()
			router.POST("/v1/sanitize", handler.Handle)

			req := httptest.NewRequest(http.MethodPost, "/v1/sanitize", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d body=%s", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				return
			}

			var resp sanitizer.Result
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}
			if tc.wantTokenized && resp.TokenizedCount == 0 {
				t.Fatalf("expected tokenized output, got prompt=%q", resp.SanitizedPrompt)
			}
			if !tc.wantTokenized && resp.TokenizedCount != 0 {
				t.Fatalf("expected redacted output (tokenized_count=0), got %d in prompt=%q", resp.TokenizedCount, resp.SanitizedPrompt)
			}
			if tc.wantPromptSubs != "" && !bytes.Contains([]byte(resp.SanitizedPrompt), []byte(tc.wantPromptSubs)) {
				t.Fatalf("expected sanitized prompt to contain %q, got %q", tc.wantPromptSubs, resp.SanitizedPrompt)
			}
		})
	}
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
