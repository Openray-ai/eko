package slm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"eko/internal/helpers/logger"
)

const (
	defaultTimeout       = 800 * time.Millisecond
	defaultMaxInputBytes = 16384
	// defaultKeepAlive sizes the per-host idle connection pool. Sized for a
	// production proxy under fan-out: many concurrent in-flight requests share
	// the pool, and undersizing forces frequent dial cost on hot paths.
	defaultKeepAlive = 32
)

// Client calls the slm-sidecar FastAPI service over HTTP/JSON. A single
// long-lived Client is intended; the underlying http.Transport keeps idle
// connections to amortize TLS/connection setup across requests.
type Client struct {
	cfg     Config
	http    *http.Client
	breaker *breaker
	metrics Metrics
}

// NewClient constructs a Client with sensible defaults applied to cfg.
func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.MaxInputBytes <= 0 {
		cfg.MaxInputBytes = defaultMaxInputBytes
	}
	if cfg.Labels == nil {
		cfg.Labels = DefaultLabels()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = noopMetrics{}
	}
	cfg.Endpoint = strings.TrimRight(cfg.Endpoint, "/")

	transport := &http.Transport{
		MaxIdleConnsPerHost:   defaultKeepAlive,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: cfg.Timeout,
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: cfg.Timeout, Transport: transport},
		breaker: newBreaker(cfg.Breaker),
		metrics: cfg.Metrics,
	}
}

// Detect is the high-level entrypoint used by the detector: runs prediction,
// applies the label mapping, and returns Violations ready to merge with regex
// hits. Returns (nil, nil) when the breaker is open or input is too large —
// the detector treats this as "no SLM contribution" and continues with regex
// results only.
func (c *Client) Detect(ctx context.Context, input string) ([]Violation, error) {
	if input == "" {
		return nil, nil
	}
	if c.cfg.MaxInputBytes > 0 && len(input) > c.cfg.MaxInputBytes {
		logger.Debug("slm: input exceeds max bytes; skipping", logger.Fields{
			"len": len(input), "max": c.cfg.MaxInputBytes,
		})
		return nil, nil
	}
	if !c.breaker.Allow(time.Now()) {
		c.metrics.IncSLMSkippedBreakerOpen()
		c.metrics.SetSLMBreakerOpen(true)
		return nil, nil
	}
	spans, err := c.Predict(ctx, input)
	if err != nil {
		return nil, err
	}
	return c.spansToViolations(spans), nil
}

// Predict calls /predict for a single string and returns raw spans.
func (c *Client) Predict(ctx context.Context, text string) ([]Span, error) {
	var resp struct {
		Spans []Span `json:"spans"`
	}
	if err := c.do(ctx, "/predict", map[string]any{"text": text}, &resp); err != nil {
		return nil, err
	}
	return resp.Spans, nil
}

// PredictBatch calls /predict_batch for multiple strings in one request.
// Results are positional with the input texts. Not used by the detector
// today; exposed so a future change in the proxy layer can collect all
// messages for a request and amortize the SLM round-trip.
func (c *Client) PredictBatch(ctx context.Context, texts []string) ([][]Span, error) {
	var resp struct {
		Results []struct {
			Spans []Span `json:"spans"`
		} `json:"results"`
	}
	if err := c.do(ctx, "/predict_batch", map[string]any{"texts": texts}, &resp); err != nil {
		return nil, err
	}
	// Enforce the positional contract: a sidecar that returns a different
	// number of results than texts breaks any caller indexing into the
	// output. Better to surface this as an error than to silently return
	// a misaligned slice.
	if len(resp.Results) != len(texts) {
		return nil, fmt.Errorf(
			"slm: predict_batch returned %d results for %d inputs",
			len(resp.Results), len(texts),
		)
	}
	out := make([][]Span, len(resp.Results))
	for i, r := range resp.Results {
		out[i] = r.Spans
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, path string, body any, dst any) error {
	started := time.Now()
	c.metrics.IncSLMRequests()

	payload, err := json.Marshal(body)
	if err != nil {
		c.recordFailure(started)
		return fmt.Errorf("slm: marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint+path, bytes.NewReader(payload))
	if err != nil {
		c.recordFailure(started)
		return fmt.Errorf("slm: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		c.recordFailure(started)
		return fmt.Errorf("slm: do request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		c.recordFailure(started)
		return fmt.Errorf("slm: status %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		c.recordFailure(started)
		return fmt.Errorf("slm: decode response: %w", err)
	}

	c.metrics.AddSLMLatency(time.Since(started))
	wasOpen := c.breaker.IsOpen()
	c.breaker.RecordSuccess()
	if wasOpen {
		c.metrics.SetSLMBreakerOpen(false)
	}
	return nil
}

func (c *Client) recordFailure(started time.Time) {
	c.metrics.IncSLMFailures()
	c.metrics.AddSLMLatency(time.Since(started))
	c.breaker.RecordFailure(time.Now())
	if c.breaker.IsOpen() {
		c.metrics.SetSLMBreakerOpen(true)
	}
}

func (c *Client) spansToViolations(spans []Span) []Violation {
	if len(spans) == 0 {
		return nil
	}
	out := make([]Violation, 0, len(spans))
	for _, s := range spans {
		m, ok := c.cfg.Labels[s.Label]
		if !ok {
			continue
		}
		out = append(out, Violation{
			Type:     m.Type,
			Severity: m.Severity,
			Pattern:  m.Pattern,
			Matched:  s.Text,
			Position: s.Start,
			End:      s.End,
		})
	}
	return out
}
