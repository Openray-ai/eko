package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBehaviorConfig(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantMode string
		wantTTL  int
		wantErr  bool
	}{
		{
			name:     "defaults when missing",
			yaml:     "proxy: {}\n",
			wantMode: "redact",
			wantTTL:  30000,
		},
		{
			name:     "valid tokenize",
			yaml:     "proxy:\n  behavior:\n    sanitization_mode: \"tokenize\"\n    token_ttl_ms: 45000\n",
			wantMode: "tokenize",
			wantTTL:  45000,
		},
		{
			name:    "invalid mode",
			yaml:    "proxy:\n  behavior:\n    sanitization_mode: \"invalid\"\n    token_ttl_ms: 30000\n",
			wantErr: true,
		},
		{
			name:    "invalid ttl",
			yaml:    "proxy:\n  behavior:\n    sanitization_mode: \"redact\"\n    token_ttl_ms: -1\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)

			cfg, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.Proxy.Behavior.SanitizationMode != tt.wantMode {
				t.Fatalf("sanitization_mode = %q, want %q", cfg.Proxy.Behavior.SanitizationMode, tt.wantMode)
			}
			if cfg.Proxy.Behavior.TokenTTLms != tt.wantTTL {
				t.Fatalf("token_ttl_ms = %d, want %d", cfg.Proxy.Behavior.TokenTTLms, tt.wantTTL)
			}
		})
	}
}

func TestDefaultBehaviorConfig(t *testing.T) {
	cfg := Default()

	if cfg.Proxy.Behavior.SanitizationMode != "redact" {
		t.Fatalf("sanitization_mode = %q, want %q", cfg.Proxy.Behavior.SanitizationMode, "redact")
	}
	if cfg.Proxy.Behavior.TokenTTLms != 30000 {
		t.Fatalf("token_ttl_ms = %d, want %d", cfg.Proxy.Behavior.TokenTTLms, 30000)
	}
	if cfg.Proxy.Behavior.TokenStoreBackend != "memory" {
		t.Fatalf("token_store_backend = %q, want %q", cfg.Proxy.Behavior.TokenStoreBackend, "memory")
	}
	if cfg.Proxy.Redis.MetaSuffix != "vault_meta:" {
		t.Fatalf("meta_suffix = %q, want %q", cfg.Proxy.Redis.MetaSuffix, "vault_meta:")
	}
	if cfg.Proxy.Redis.PayloadSuffix != "vault_payload:" {
		t.Fatalf("payload_suffix = %q, want %q", cfg.Proxy.Redis.PayloadSuffix, "vault_payload:")
	}
	if cfg.Proxy.Crypto.Provider != "local" {
		t.Fatalf("crypto.provider = %q, want %q", cfg.Proxy.Crypto.Provider, "local")
	}
	if cfg.Proxy.Crypto.VaultTransit.Mount != "transit" {
		t.Fatalf("vault_transit.mount = %q, want %q", cfg.Proxy.Crypto.VaultTransit.Mount, "transit")
	}
	if cfg.Proxy.Crypto.VaultTransit.KeyName != "eko-session" {
		t.Fatalf("vault_transit.key_name = %q, want %q", cfg.Proxy.Crypto.VaultTransit.KeyName, "eko-session")
	}
}

func TestLoadBehaviorConfig_InvalidBackend(t *testing.T) {
	path := writeTempConfig(t, "proxy:\n  behavior:\n    token_store_backend: \"bad\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected invalid backend to fail")
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return path
}
