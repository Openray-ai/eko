package tokenizer

import "testing"

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
