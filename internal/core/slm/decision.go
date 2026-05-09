package slm

import "context"

// Per-request SLM opt-in/opt-out, propagated through context.Context.
//
// The config-level `proxy.slm.enabled` flag controls whether the SLM client
// is wired up at process start. This per-request flag lets a single caller
// (currently the public POST /v1/sanitize handler) override that decision
// for a specific request without affecting any other path.
//
// When the value is unset on a context, the detector falls back to its
// default behavior: run the SLM if a runner is configured. The OpenAI
// proxy paths intentionally don't set this so they preserve current
// behavior.

type contextKey struct{}

// WithRequestEnabled returns a context that explicitly opts a request in to
// (or out of) SLM detection. Pass the result to Detector.DetectWithContext.
func WithRequestEnabled(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, contextKey{}, enabled)
}

// RequestDecision reports whether the caller has explicitly opted the
// request in to SLM. set==false means no decision was provided and the
// detector's default (run SLM if configured) applies.
func RequestDecision(ctx context.Context) (enabled, set bool) {
	v, ok := ctx.Value(contextKey{}).(bool)
	if !ok {
		return false, false
	}
	return v, true
}
