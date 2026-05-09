package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetricsHandler_RendersSLMCounters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	col := NewMetricsCollector()
	// Seed some traffic so the rendered values aren't all zero.
	col.IncSLMRequests()
	col.IncSLMRequests()
	col.IncSLMFailures()
	col.IncSLMSkippedBreakerOpen()
	col.AddSLMLatency(120 * time.Millisecond)
	col.AddSLMLatency(80 * time.Millisecond)
	col.SetSLMBreakerOpen(true)

	router := gin.New()
	router.GET("/metrics", NewMetricsHandler(col).Handle)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{
		"eko_slm_requests_total 2",
		"eko_slm_failures_total 1",
		"eko_slm_skipped_breaker_open_total 1",
		"eko_slm_breaker_open 1",
		// Latency is rendered as a float; just confirm the metric line exists
		// with a non-zero value rather than pinning the exact average.
		"eko_slm_latency_avg_ms 100.00",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestMetricsHandler_DefaultZeroSLMCounters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	col := NewMetricsCollector()
	router := gin.New()
	router.GET("/metrics", NewMetricsHandler(col).Handle)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		"eko_slm_requests_total 0",
		"eko_slm_failures_total 0",
		"eko_slm_breaker_open 0",
		"eko_slm_latency_avg_ms 0.00",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q\n--- body ---\n%s", want, body)
		}
	}
}
