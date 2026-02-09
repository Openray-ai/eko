package tokenizer

import "testing"

func TestGenerateSessionID(t *testing.T) {
	sessionID := GenerateSessionID()
	if err := ValidateSessionID(sessionID); err != nil {
		t.Fatalf("expected generated session id to be valid, got error: %v", err)
	}
}

func TestGenerateSessionID_Unique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		sessionID := GenerateSessionID()
		if err := ValidateSessionID(sessionID); err != nil {
			t.Fatalf("expected generated session id to be valid, got error: %v", err)
		}
		if _, exists := seen[sessionID]; exists {
			t.Fatalf("expected generated session id to be unique, got duplicate: %s", sessionID)
		}
		seen[sessionID] = struct{}{}
	}
}

func TestValidateSessionID(t *testing.T) {
	validSessionID := "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
	}{
		{
			name:      "valid session id",
			sessionID: validSessionID,
			wantErr:   false,
		},
		{
			name:      "empty session id",
			sessionID: "",
			wantErr:   true,
		},
		{
			name:      "missing prefix",
			sessionID: "a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c",
			wantErr:   true,
		},
		{
			name:      "prefix only",
			sessionID: "eko_",
			wantErr:   true,
		},
		{
			name:      "invalid uuid length",
			sessionID: "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5",
			wantErr:   true,
		},
		{
			name:      "invalid uuid character",
			sessionID: "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5z",
			wantErr:   true,
		},
		{
			name:      "invalid uuid hyphen positions",
			sessionID: "eko_a7f3b2c14d5e-6f7a-8b9c-0d1e2f3a4b5c",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionID(tt.sessionID)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.wantErr && err != ErrInvalidSessionID {
				t.Fatalf("expected ErrInvalidSessionID, got %v", err)
			}
		})
	}
}
