package tokenizer

import "context"

type SessionHandle interface {
	Vault() *Vault
	Save(ctx context.Context) error
}

// SessionStore persists tokenization vaults across requests.
//
// TTL semantics: BeginSession, GetSession, and HasSession all refresh the
// session TTL ("sliding window"). Callers that need to probe for existence
// without extending lifetime should not use HasSession in a hot path; in the
// resolver, HasSession is only called when we already plan to read the
// session, so the touch is effectively free.
type SessionStore interface {
	BeginSession(ctx context.Context, sessionID string) (SessionHandle, error)
	GetSession(ctx context.Context, sessionID string) (*Vault, error)
	HasSession(ctx context.Context, sessionID string) (bool, error)
	DeleteSession(ctx context.Context, sessionID string) error
	Close() error
}

type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// ConflictRetrier reports how many tokenization attempts the sanitizer should
// make against this store before surfacing ErrSessionStoreConflict. Stores
// that cannot conflict (e.g. in-memory) should not implement this interface,
// in which case the sanitizer runs a single attempt.
type ConflictRetrier interface {
	ConflictRetries() int
}
