package handlers

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// MetricsCollector tracks request and sanitization counters.
// It is safe for concurrent use via atomic operations.
type MetricsCollector struct {
	startTime time.Time

	requestsTotal      atomic.Int64
	sanitizationsTotal atomic.Int64
	violationsTotal    atomic.Int64
	errorsTotal        atomic.Int64
}

// NewMetricsCollector returns a MetricsCollector with its start time set to now.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{startTime: time.Now()}
}

func (m *MetricsCollector) IncRequests()     { m.requestsTotal.Add(1) }
func (m *MetricsCollector) IncSanitizations() { m.sanitizationsTotal.Add(1) }
func (m *MetricsCollector) AddViolations(n int64) { m.violationsTotal.Add(n) }
func (m *MetricsCollector) IncErrors()       { m.errorsTotal.Add(1) }

// MetricsHandler serves Prometheus-compatible plaintext metrics at GET /metrics.
type MetricsHandler struct {
	collector *MetricsCollector
}

func NewMetricsHandler(c *MetricsCollector) *MetricsHandler {
	return &MetricsHandler{collector: c}
}

func (h *MetricsHandler) Handle(c *gin.Context) {
	col := h.collector
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptimeSeconds := time.Since(col.startTime).Seconds()

	body := fmt.Sprintf(`# HELP eko_requests_total Total number of HTTP requests received
# TYPE eko_requests_total counter
eko_requests_total %d

# HELP eko_sanitizations_total Total number of sanitization operations performed
# TYPE eko_sanitizations_total counter
eko_sanitizations_total %d

# HELP eko_violations_total Total number of violations detected across all sanitizations
# TYPE eko_violations_total counter
eko_violations_total %d

# HELP eko_errors_total Total number of errors encountered during sanitization
# TYPE eko_errors_total counter
eko_errors_total %d

# HELP eko_uptime_seconds Number of seconds since the process started
# TYPE eko_uptime_seconds gauge
eko_uptime_seconds %.2f

# HELP eko_goroutines Current number of goroutines
# TYPE eko_goroutines gauge
eko_goroutines %d

# HELP eko_memory_alloc_bytes Currently allocated heap memory in bytes
# TYPE eko_memory_alloc_bytes gauge
eko_memory_alloc_bytes %d

# HELP eko_memory_sys_bytes Total memory obtained from the OS in bytes
# TYPE eko_memory_sys_bytes gauge
eko_memory_sys_bytes %d
`,
		col.requestsTotal.Load(),
		col.sanitizationsTotal.Load(),
		col.violationsTotal.Load(),
		col.errorsTotal.Load(),
		uptimeSeconds,
		runtime.NumGoroutine(),
		mem.Alloc,
		mem.Sys,
	)

	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(body))
}
