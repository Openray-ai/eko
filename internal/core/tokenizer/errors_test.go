package tokenizer

import (
	"errors"
	"testing"
	"time"

	"eko/internal/core/detector"
)

func TestErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		message string
	}{
		{
			name:    "session expired",
			err:     ErrSessionExpired,
			message: "session expired",
		},
		{
			name:    "vault not found",
			err:     ErrVaultNotFound,
			message: "vault not found",
		},
		{
			name:    "vault full",
			err:     ErrVaultFull,
			message: "vault full",
		},
		{
			name:    "invalid session id",
			err:     ErrInvalidSessionID,
			message: "invalid session id",
		},
		{
			name:    "token length exceeded",
			err:     ErrTokenLengthExceeded,
			message: "token length exceeded",
		},
		{
			name:    "credential tokenization not allowed",
			err:     ErrCredentialTokenization,
			message: "credential tokenization not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("expected error to be non-nil")
			}
			if tt.err.Error() != tt.message {
				t.Fatalf("error message = %q, want %q", tt.err.Error(), tt.message)
			}
		})
	}
}

func TestErrVaultFullWhenMaxVaultsExceeded(t *testing.T) {
	manager := NewVaultManager(time.Minute, WithMaxVaults(1))

	if _, err := manager.GetOrCreate("eko_11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("expected first vault creation to succeed, got error: %v", err)
	}

	_, err := manager.GetOrCreate("eko_22222222-2222-2222-2222-222222222222")
	if !errors.Is(err, ErrVaultFull) {
		t.Fatalf("expected ErrVaultFull, got %v", err)
	}
}

func TestErrVaultFullWhenMaxTokensExceeded(t *testing.T) {
	manager := NewVaultManager(time.Minute, WithMaxTokensPerVault(1))
	vault, err := manager.GetOrCreate("eko_33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	tokenizer := NewTokenizer()
	first := detector.Violation{Matched: "john@acme.com", Pattern: "email"}
	second := detector.Violation{Matched: "jane@acme.com", Pattern: "email"}

	if _, err := tokenizer.Generate(first, vault); err != nil {
		t.Fatalf("expected first tokenization to succeed, got error: %v", err)
	}

	_, err = tokenizer.Generate(second, vault)
	if !errors.Is(err, ErrVaultFull) {
		t.Fatalf("expected ErrVaultFull, got %v", err)
	}

	if size := vault.Size(); size != 1 {
		t.Fatalf("vault size = %d, want %d", size, 1)
	}

	vault.mu.RLock()
	forwardLen := len(vault.tokens.forward)
	reverseLen := len(vault.tokens.reverse)
	vault.mu.RUnlock()

	if forwardLen != reverseLen {
		t.Fatalf("forward entries = %d, reverse entries = %d", forwardLen, reverseLen)
	}
}
