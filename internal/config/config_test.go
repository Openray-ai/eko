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
			if cfg.Proxy.Behavior.MaxBatchItems != 100 {
				t.Fatalf("max_batch_items = %d, want %d", cfg.Proxy.Behavior.MaxBatchItems, 100)
			}
			if cfg.Proxy.Behavior.MaxPromptBytes != 65536 {
				t.Fatalf("max_prompt_bytes = %d, want %d", cfg.Proxy.Behavior.MaxPromptBytes, 65536)
			}
			if cfg.Proxy.Behavior.MaxBatchBytes != 1048576 {
				t.Fatalf("max_batch_bytes = %d, want %d", cfg.Proxy.Behavior.MaxBatchBytes, 1048576)
			}
			if cfg.Proxy.Behavior.MaxBatchConcurrency != 1 {
				t.Fatalf("max_batch_concurrency = %d, want %d", cfg.Proxy.Behavior.MaxBatchConcurrency, 1)
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
	if cfg.Proxy.Behavior.MaxBatchItems != 100 {
		t.Fatalf("max_batch_items = %d, want %d", cfg.Proxy.Behavior.MaxBatchItems, 100)
	}
	if cfg.Proxy.Behavior.MaxPromptBytes != 65536 {
		t.Fatalf("max_prompt_bytes = %d, want %d", cfg.Proxy.Behavior.MaxPromptBytes, 65536)
	}
	if cfg.Proxy.Behavior.MaxBatchBytes != 1048576 {
		t.Fatalf("max_batch_bytes = %d, want %d", cfg.Proxy.Behavior.MaxBatchBytes, 1048576)
	}
	if cfg.Proxy.Behavior.MaxBatchConcurrency != 1 {
		t.Fatalf("max_batch_concurrency = %d, want %d", cfg.Proxy.Behavior.MaxBatchConcurrency, 1)
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

func TestLoadBehaviorConfig_BatchLimitOverrides(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  behavior:
    max_batch_items: 7
    max_prompt_bytes: 128
    max_batch_bytes: 2048
    max_batch_concurrency: 2
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Behavior.MaxBatchItems != 7 {
		t.Fatalf("max_batch_items = %d, want %d", cfg.Proxy.Behavior.MaxBatchItems, 7)
	}
	if cfg.Proxy.Behavior.MaxPromptBytes != 128 {
		t.Fatalf("max_prompt_bytes = %d, want %d", cfg.Proxy.Behavior.MaxPromptBytes, 128)
	}
	if cfg.Proxy.Behavior.MaxBatchBytes != 2048 {
		t.Fatalf("max_batch_bytes = %d, want %d", cfg.Proxy.Behavior.MaxBatchBytes, 2048)
	}
	if cfg.Proxy.Behavior.MaxBatchConcurrency != 2 {
		t.Fatalf("max_batch_concurrency = %d, want %d", cfg.Proxy.Behavior.MaxBatchConcurrency, 2)
	}
}

func TestLoadBehaviorConfig_InvalidBatchLimits(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"invalid max_batch_items", "proxy:\n  behavior:\n    max_batch_items: -1\n"},
		{"invalid max_prompt_bytes", "proxy:\n  behavior:\n    max_prompt_bytes: -1\n"},
		{"invalid max_batch_bytes", "proxy:\n  behavior:\n    max_batch_bytes: -1\n"},
		{"invalid max_batch_concurrency", "proxy:\n  behavior:\n    max_batch_concurrency: -1\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid batch limit to fail")
			}
		})
	}
}

func TestLoadSLMConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(*testing.T, *Config)
	}{
		{
			name: "disabled by default applies no validation",
			yaml: "proxy: {}\n",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Proxy.SLM.Enabled {
					t.Fatalf("expected slm.enabled=false by default")
				}
				if cfg.Proxy.SLM.Endpoint != "http://slm-sidecar:8000" {
					t.Fatalf("expected default endpoint, got %q", cfg.Proxy.SLM.Endpoint)
				}
				if cfg.Proxy.SLM.TimeoutMs != 800 {
					t.Fatalf("expected default timeout 800ms, got %d", cfg.Proxy.SLM.TimeoutMs)
				}
				if cfg.Proxy.SLM.Breaker.FailureThreshold != 5 {
					t.Fatalf("expected default failure_threshold 5, got %d", cfg.Proxy.SLM.Breaker.FailureThreshold)
				}
			},
		},
		{
			name: "enabled with overrides",
			yaml: "proxy:\n  slm:\n    enabled: true\n    endpoint: \"http://x:9000\"\n    timeout_ms: 250\n    breaker:\n      failure_threshold: 3\n      cooldown_ms: 5000\n    labels:\n      private_phone:\n        severity: \"BLOCK\"\n",
			check: func(t *testing.T, cfg *Config) {
				if !cfg.Proxy.SLM.Enabled {
					t.Fatalf("expected enabled=true")
				}
				if cfg.Proxy.SLM.Endpoint != "http://x:9000" {
					t.Fatalf("endpoint mismatch: %q", cfg.Proxy.SLM.Endpoint)
				}
				if cfg.Proxy.SLM.TimeoutMs != 250 {
					t.Fatalf("timeout mismatch: %d", cfg.Proxy.SLM.TimeoutMs)
				}
				ov, ok := cfg.Proxy.SLM.Labels["private_phone"]
				if !ok || ov.Severity != "BLOCK" {
					t.Fatalf("expected label override BLOCK, got %+v", ov)
				}
			},
		},
		{
			name:    "enabled with invalid severity rejected",
			yaml:    "proxy:\n  slm:\n    enabled: true\n    endpoint: \"http://x:9000\"\n    labels:\n      private_phone:\n        severity: \"NUCLEAR\"\n",
			wantErr: true,
		},
		{
			name:    "enabled with negative timeout rejected",
			yaml:    "proxy:\n  slm:\n    enabled: true\n    endpoint: \"http://x:9000\"\n    timeout_ms: -1\n",
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
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
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

func TestApplyEnvOverrides(t *testing.T) {
	t.Run("PORT overrides server.port", func(t *testing.T) {
		t.Setenv("PORT", "9090")
		cfg := Default()
		applyEnvOverrides(cfg)
		if cfg.Server.Port != "9090" {
			t.Fatalf("port = %q, want 9090", cfg.Server.Port)
		}
	})

	t.Run("EKO_SLM_ENDPOINT overrides slm.endpoint", func(t *testing.T) {
		t.Setenv("EKO_SLM_ENDPOINT", "http://eko-slm.flycast")
		cfg := Default()
		cfg.Proxy.SLM.Endpoint = "http://slm-sidecar:8000"
		applyEnvOverrides(cfg)
		if cfg.Proxy.SLM.Endpoint != "http://eko-slm.flycast" {
			t.Fatalf("slm endpoint = %q, want http://eko-slm.flycast", cfg.Proxy.SLM.Endpoint)
		}
	})

	t.Run("empty env vars leave config untouched", func(t *testing.T) {
		t.Setenv("PORT", "")
		t.Setenv("EKO_SLM_ENDPOINT", "")

		cfg := Default()
		cfg.Server.Port = "8080"
		cfg.Proxy.SLM.Endpoint = "http://slm-sidecar:8000"
		applyEnvOverrides(cfg)

		if cfg.Server.Port != "8080" {
			t.Fatalf("port = %q, want 8080", cfg.Server.Port)
		}
		if cfg.Proxy.SLM.Endpoint != "http://slm-sidecar:8000" {
			t.Fatalf("endpoint changed: %q", cfg.Proxy.SLM.Endpoint)
		}
	})
}
