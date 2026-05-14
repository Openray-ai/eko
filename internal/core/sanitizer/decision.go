package sanitizer

import "context"

// Per-request sanitization mode override, propagated through context.Context.
//
// The config-level `proxy.behavior.sanitization_mode` flag fixes the
// sanitizer's default mode at process start. This per-request override lets
// the public POST /v1/sanitize handler flip the mode for a specific request
// without affecting any other caller. The OpenAI proxy paths intentionally
// don't set this so they preserve current behavior.

type modeContextKey struct{}

// WithRequestMode returns a context that explicitly selects a sanitization
// mode ("redact" or "tokenize") for a single request, overriding the
// sanitizer's configured default. An empty mode is treated as "no override"
// and the original context is returned unchanged.
//
// Callers are expected to validate the mode value at the request boundary;
// this helper trusts its input.
func WithRequestMode(ctx context.Context, mode string) context.Context {
	if mode == "" {
		return ctx
	}
	return context.WithValue(ctx, modeContextKey{}, mode)
}

// RequestMode reports whether the caller has set an explicit per-request
// mode. set==false means no override was provided and the sanitizer's
// configured default applies.
func RequestMode(ctx context.Context) (mode string, set bool) {
	v, ok := ctx.Value(modeContextKey{}).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}
