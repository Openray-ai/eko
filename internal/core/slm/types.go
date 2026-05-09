package slm

import "time"

// Span is the raw token-classification output from the sidecar.
type Span struct {
	Label string  `json:"label"`
	Text  string  `json:"text"`
	Start int     `json:"start"`
	End   int     `json:"end"`
	Score float64 `json:"score"`
}

// LabelMapping converts an SLM label into Ekō Violation fields.
type LabelMapping struct {
	Type     string
	Severity string
	Pattern  string
}

// Violation mirrors detector.Violation in shape but lives here to avoid an
// import cycle. The detector converts these at the boundary.
type Violation struct {
	Type     string
	Severity string
	Pattern  string
	Matched  string
	Position int
	End      int
}

// Metrics is implemented by the metrics collector. Decouples slm from the
// handlers package so this package can be tested in isolation.
type Metrics interface {
	IncSLMRequests()
	IncSLMFailures()
	IncSLMSkippedBreakerOpen()
	AddSLMLatency(d time.Duration)
	SetSLMBreakerOpen(open bool)
}

type noopMetrics struct{}

func (noopMetrics) IncSLMRequests()             {}
func (noopMetrics) IncSLMFailures()             {}
func (noopMetrics) IncSLMSkippedBreakerOpen()   {}
func (noopMetrics) AddSLMLatency(time.Duration) {}
func (noopMetrics) SetSLMBreakerOpen(bool)      {}

// BreakerConfig configures the in-process circuit breaker.
type BreakerConfig struct {
	FailureThreshold int
	Cooldown         time.Duration
}

// Config is the construction config for Client.
type Config struct {
	Endpoint      string
	Timeout       time.Duration
	MaxInputBytes int
	Labels        map[string]LabelMapping
	Breaker       BreakerConfig
	Metrics       Metrics
}
