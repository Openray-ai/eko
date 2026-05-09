package slm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Predict_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["text"] != "hello" {
			t.Fatalf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"spans":[{"label":"private_phone","text":"+234","start":3,"end":7,"score":0.9}]}`)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, Timeout: time.Second})

	spans, err := client.Predict(context.Background(), "hello")
	if err != nil {
		t.Fatalf("predict: %v", err)
	}
	if len(spans) != 1 || spans[0].Label != "private_phone" {
		t.Fatalf("unexpected spans: %+v", spans)
	}
}

func TestClient_Detect_AppliesMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `{"spans":[
            {"label":"private_phone","text":"+234","start":0,"end":4,"score":0.9},
            {"label":"unknown_label","text":"x","start":5,"end":6,"score":0.5}
        ]}`)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, Timeout: time.Second})
	violations, err := client.Detect(context.Background(), "phone is +234")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected one violation (unknown_label dropped), got %d: %+v", len(violations), violations)
	}
	if violations[0].Pattern != "slm_phone" {
		t.Fatalf("expected pattern slm_phone, got %q", violations[0].Pattern)
	}
	if violations[0].Severity != "WARN" {
		t.Fatalf("expected severity WARN, got %q", violations[0].Severity)
	}
}

func TestClient_PredictBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict_batch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprintln(w, `{"results":[
            {"spans":[{"label":"private_email","text":"a@b","start":0,"end":3,"score":0.7}]},
            {"spans":[]}
        ]}`)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, Timeout: time.Second})
	batches, err := client.PredictBatch(context.Background(), []string{"a@b", ""})
	if err != nil {
		t.Fatalf("predict_batch: %v", err)
	}
	if len(batches) != 2 || len(batches[0]) != 1 || batches[0][0].Label != "private_email" {
		t.Fatalf("unexpected batches: %+v", batches)
	}
}

func TestClient_PredictBatch_RejectsCardinalityMismatch(t *testing.T) {
	// Sidecar returns one result for two inputs — caller would silently
	// receive a misaligned slice if we didn't enforce positional contract.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `{"results":[{"spans":[]}]}`)
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, Timeout: time.Second})
	_, err := client.PredictBatch(context.Background(), []string{"one", "two"})
	if err == nil {
		t.Fatalf("expected cardinality error")
	}
	if !strings.Contains(err.Error(), "predict_batch returned") {
		t.Fatalf("error should mention cardinality, got: %v", err)
	}
}

type countingMetrics struct {
	requests, failures, breakerSkips atomic.Int64
	breakerOpen                      atomic.Bool
	latency                          atomic.Int64
}

func (m *countingMetrics) IncSLMRequests()               { m.requests.Add(1) }
func (m *countingMetrics) IncSLMFailures()               { m.failures.Add(1) }
func (m *countingMetrics) IncSLMSkippedBreakerOpen()     { m.breakerSkips.Add(1) }
func (m *countingMetrics) AddSLMLatency(d time.Duration) { m.latency.Add(int64(d)) }
func (m *countingMetrics) SetSLMBreakerOpen(open bool)   { m.breakerOpen.Store(open) }

func TestClient_Detect_BreakerOpensAfterFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"error":"boom"}`)
	}))
	defer server.Close()

	metrics := &countingMetrics{}
	client := NewClient(Config{
		Endpoint: server.URL,
		Timeout:  time.Second,
		Breaker:  BreakerConfig{FailureThreshold: 2, Cooldown: 50 * time.Millisecond},
		Metrics:  metrics,
	})

	for i := range 2 {
		if _, err := client.Detect(context.Background(), "x"); err == nil {
			t.Fatalf("expected error on iter %d", i)
		}
	}
	out, err := client.Detect(context.Background(), "x")
	if err != nil || out != nil {
		t.Fatalf("expected breaker-open skip, got out=%v err=%v", out, err)
	}
	if metrics.breakerSkips.Load() == 0 {
		t.Fatalf("expected breakerSkips > 0")
	}
	if !metrics.breakerOpen.Load() {
		t.Fatalf("expected breakerOpen=true")
	}

	time.Sleep(80 * time.Millisecond)
	if !client.breaker.Allow(time.Now()) {
		t.Fatalf("breaker should allow after cooldown")
	}
}

func TestClient_Detect_TimeoutCountedAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"spans":[]}`)
	}))
	defer server.Close()

	metrics := &countingMetrics{}
	client := NewClient(Config{
		Endpoint: server.URL,
		Timeout:  20 * time.Millisecond,
		Metrics:  metrics,
	})
	if _, err := client.Detect(context.Background(), "x"); err == nil {
		t.Fatalf("expected timeout error")
	}
	if metrics.failures.Load() != 1 {
		t.Fatalf("expected 1 failure, got %d", metrics.failures.Load())
	}
}

func TestClient_Detect_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"spans": notjson}`))
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, Timeout: time.Second})
	if _, err := client.Detect(context.Background(), "x"); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestClient_Detect_SkipsOversizeInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatalf("server should not be called for oversize input")
	}))
	defer server.Close()

	client := NewClient(Config{Endpoint: server.URL, Timeout: time.Second, MaxInputBytes: 4})
	out, err := client.Detect(context.Background(), strings.Repeat("a", 100))
	if err != nil || out != nil {
		t.Fatalf("expected silent skip, got out=%v err=%v", out, err)
	}
}

func TestMergeLabelOverrides(t *testing.T) {
	defaults := DefaultLabels()
	overrides := map[string]LabelMapping{
		"private_phone": {Severity: "BLOCK"},
		"custom_label":  {Type: "pii", Severity: "LOG", Pattern: "slm_custom"},
	}
	merged := MergeLabelOverrides(defaults, overrides)
	if merged["private_phone"].Severity != "BLOCK" {
		t.Fatalf("override should change severity")
	}
	if merged["private_phone"].Pattern != "slm_phone" {
		t.Fatalf("non-overridden field should remain default")
	}
	if merged["custom_label"].Pattern != "slm_custom" {
		t.Fatalf("new label should be added")
	}
}
