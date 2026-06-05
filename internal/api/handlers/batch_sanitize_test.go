package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/sanitizer"
	"eko/internal/core/slm"
	"eko/internal/core/tokenizer"

	"github.com/gin-gonic/gin"
)

func TestBatchSanitizeHandler_ValidBatch(t *testing.T) {
	router := newBatchTestRouter(t, "redact", BatchSanitizeLimits{})

	body := `{"items":[{"id":"one","prompt":"My email is john.doe@example.com"},{"id":"two","prompt":"Account ACCT1234"}]}`
	rec := postBatch(router, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp BatchSanitizeResponse
	decodeResponse(t, rec, &resp)
	if resp.Summary.Total != 2 || resp.Summary.Succeeded != 2 || resp.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if resp.Results[0].ID != "one" || !resp.Results[0].OK || resp.Results[0].Result == nil {
		t.Fatalf("unexpected first result: %+v", resp.Results[0])
	}
	if resp.Results[0].Result.OriginalPrompt != "" {
		t.Fatalf("expected batch result to omit original_prompt, got %q", resp.Results[0].Result.OriginalPrompt)
	}
	if strings.Contains(resp.Results[0].Result.SanitizedPrompt, "john.doe@example.com") {
		t.Fatalf("expected email to be redacted: %q", resp.Results[0].Result.SanitizedPrompt)
	}
}

func TestBatchSanitizeHandler_MixedSuccessAndFailure(t *testing.T) {
	router := newBatchTestRouter(t, "redact", BatchSanitizeLimits{})

	body := `{"items":[{"id":"good","prompt":"My email is john.doe@example.com"},{"id":"bad","prompt":"hello","sanitization_mode":"mask"}]}`
	rec := postBatch(router, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp BatchSanitizeResponse
	decodeResponse(t, rec, &resp)
	if resp.Summary.Succeeded != 1 || resp.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if !resp.Results[0].OK {
		t.Fatalf("expected first item to succeed: %+v", resp.Results[0])
	}
	if resp.Results[1].OK || resp.Results[1].Error == nil || resp.Results[1].Error.Code != "invalid_sanitization_mode" {
		t.Fatalf("expected invalid_sanitization_mode, got %+v", resp.Results[1])
	}
}

func TestBatchSanitizeHandler_EnvelopeValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		limits     BatchSanitizeLimits
		wantStatus int
	}{
		{
			name:       "missing items",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty items",
			body:       `{"items":[]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "too many items",
			body:       `{"items":[{"prompt":"a"},{"prompt":"b"}]}`,
			limits:     BatchSanitizeLimits{MaxBatchItems: 1},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "oversized body",
			body:       `{"items":[{"prompt":"` + strings.Repeat("a", 80) + `"}]}`,
			limits:     BatchSanitizeLimits{MaxBatchBytes: 20},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "trailing json",
			body:       `{"items":[{"prompt":"a"}]} {"items":[{"prompt":"b"}]}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newBatchTestRouter(t, "redact", tt.limits)
			rec := postBatch(router, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBatchSanitizeHandler_ItemValidation(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		limits   BatchSanitizeLimits
		wantCode string
	}{
		{
			name:     "missing prompt",
			body:     `{"items":[{"id":"x"}]}`,
			wantCode: "missing_prompt",
		},
		{
			name:     "invalid session id",
			body:     `{"items":[{"id":"x","prompt":"hello","session_id":"bad"}]}`,
			wantCode: "invalid_session_id",
		},
		{
			name:     "prompt too large",
			body:     `{"items":[{"id":"x","prompt":"abcdef"}]}`,
			limits:   BatchSanitizeLimits{MaxPromptBytes: 5},
			wantCode: "prompt_too_large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newBatchTestRouter(t, "redact", tt.limits)
			rec := postBatch(router, tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}

			var resp BatchSanitizeResponse
			decodeResponse(t, rec, &resp)
			if resp.Summary.Succeeded != 0 || resp.Summary.Failed != 1 {
				t.Fatalf("unexpected summary: %+v", resp.Summary)
			}
			if resp.Results[0].Error == nil || resp.Results[0].Error.Code != tt.wantCode {
				t.Fatalf("expected %q, got %+v", tt.wantCode, resp.Results[0])
			}
		})
	}
}

func TestBatchSanitizeHandler_TokenizeGeneratedAndSharedSession(t *testing.T) {
	router := newBatchTestRouter(t, "redact", BatchSanitizeLimits{})
	sessionID := tokenizer.GenerateSessionID()

	body := `{"items":[` +
		`{"id":"generated","prompt":"My email is john.doe@example.com","sanitization_mode":"tokenize"},` +
		`{"id":"shared-1","prompt":"My email is jane@example.com","session_id":"` + sessionID + `","sanitization_mode":"tokenize"},` +
		`{"id":"shared-2","prompt":"My email is jane@example.com","session_id":"` + sessionID + `","sanitization_mode":"tokenize"}` +
		`]}`
	rec := postBatch(router, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp BatchSanitizeResponse
	decodeResponse(t, rec, &resp)
	if resp.Summary.Succeeded != 3 || resp.Summary.Tokenized != 3 {
		t.Fatalf("unexpected summary: %+v", resp.Summary)
	}
	if resp.Results[0].Result.SessionID == "" {
		t.Fatal("expected generated session id")
	}
	if resp.Results[1].Result.SessionID != sessionID || resp.Results[2].Result.SessionID != sessionID {
		t.Fatalf("expected shared session id %q, got %q and %q", sessionID, resp.Results[1].Result.SessionID, resp.Results[2].Result.SessionID)
	}
	if resp.Results[1].Result.SanitizedPrompt != resp.Results[2].Result.SanitizedPrompt {
		t.Fatalf("expected stable token reuse, got %q and %q", resp.Results[1].Result.SanitizedPrompt, resp.Results[2].Result.SanitizedPrompt)
	}
}

type batchStubSLM struct {
	calls int
}

func (s *batchStubSLM) Detect(_ context.Context, _ string) ([]slm.Violation, error) {
	s.calls++
	return nil, nil
}

func TestBatchSanitizeHandler_SLMOptInPerItem(t *testing.T) {
	gin.SetMode(gin.TestMode)
	det := detector.New()
	loadTestPatterns(t, det)
	stub := &batchStubSLM{}
	det.SetSLM(stub)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	san := sanitizer.NewWithTokenizer(det, tok, vm, "redact")
	handler := NewBatchSanitizeHandler(san, BatchSanitizeLimits{}, nil)

	router := gin.New()
	router.POST("/v1/sanitize/batch", handler.Handle)
	rec := postBatch(router, `{"items":[{"prompt":"hello","slm":false},{"prompt":"hello","slm":true}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if stub.calls != 1 {
		t.Fatalf("expected one SLM call, got %d", stub.calls)
	}
}

func TestBatchSanitizeHandler_SessionStoreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	det := detector.New()
	loadTestPatterns(t, det)

	tok := tokenizer.NewTokenizer()
	store := &batchFailingSessionStore{beginErr: tokenizer.ErrSessionStoreUnavailable}
	san := sanitizer.NewWithTokenizer(det, tok, store, "tokenize")
	handler := NewBatchSanitizeHandler(san, BatchSanitizeLimits{}, nil)

	router := gin.New()
	router.POST("/v1/sanitize/batch", handler.Handle)
	rec := postBatch(router, `{"items":[{"prompt":"My email is john.doe@example.com","sanitization_mode":"tokenize"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp BatchSanitizeResponse
	decodeResponse(t, rec, &resp)
	if resp.Results[0].Error == nil || resp.Results[0].Error.Code != "session_store_unavailable" {
		t.Fatalf("expected session_store_unavailable, got %+v", resp.Results[0])
	}
}

func newBatchTestRouter(t *testing.T, mode string, limits BatchSanitizeLimits) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	det := detector.New()
	loadTestPatterns(t, det)
	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	san := sanitizer.NewWithTokenizer(det, tok, vm, mode)

	router := gin.New()
	router.POST("/v1/sanitize/batch", NewBatchSanitizeHandler(san, limits, NewMetricsCollector()).Handle)
	return router
}

func postBatch(router *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/sanitize/batch", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("failed to unmarshal response: %v body=%s", err, rec.Body.String())
	}
}

type batchFailingSessionStore struct {
	beginErr error
}

func (s *batchFailingSessionStore) BeginSession(context.Context, string) (tokenizer.SessionHandle, error) {
	return nil, s.beginErr
}

func (s *batchFailingSessionStore) GetSession(context.Context, string) (*tokenizer.Vault, error) {
	return nil, tokenizer.ErrVaultNotFound
}

func (s *batchFailingSessionStore) HasSession(context.Context, string) (bool, error) {
	return false, nil
}

func (s *batchFailingSessionStore) DeleteSession(context.Context, string) error {
	return nil
}

func (s *batchFailingSessionStore) Close() error {
	return nil
}
