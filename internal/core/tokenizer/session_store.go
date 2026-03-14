package tokenizer

import "context"

type SessionHandle interface {
	Vault() *Vault
	Save(ctx context.Context) error
}

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

type ConflictRetrier interface {
	ConflictRetries() int
}
