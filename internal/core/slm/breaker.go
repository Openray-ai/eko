package slm

import (
	"sync"
	"sync/atomic"
	"time"
)

// breaker is a minimal in-process circuit breaker. It opens after
// FailureThreshold consecutive failures and resets after Cooldown elapses on
// the next Allow check. No background goroutines, no third-party deps.
type breaker struct {
	threshold int
	cooldown  time.Duration

	mu        sync.Mutex
	failures  int
	openUntil time.Time

	open atomic.Bool
}

func newBreaker(cfg BreakerConfig) *breaker {
	threshold := cfg.FailureThreshold
	if threshold < 1 {
		threshold = 5
	}
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &breaker{threshold: threshold, cooldown: cooldown}
}

// Allow reports whether a call should proceed. When open and the cooldown has
// elapsed, the breaker is reset and the call is allowed to attempt recovery.
func (b *breaker) Allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() || !now.Before(b.openUntil) {
		if b.open.Load() {
			b.open.Store(false)
			b.failures = 0
			b.openUntil = time.Time{}
		}
		return true
	}
	return false
}

func (b *breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	if b.open.Load() {
		b.open.Store(false)
		b.openUntil = time.Time{}
	}
}

func (b *breaker) RecordFailure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold && !b.open.Load() {
		b.open.Store(true)
		b.openUntil = now.Add(b.cooldown)
	}
}

func (b *breaker) IsOpen() bool {
	return b.open.Load()
}
