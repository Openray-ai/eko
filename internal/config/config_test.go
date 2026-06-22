package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if cfg.Proxy.DeepSeek.BaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("deepseek.base_url = %q", cfg.Proxy.DeepSeek.BaseURL)
	}
	if cfg.Proxy.ModelRouting.DefaultProvider != "openai" {
		t.Fatalf("default_provider = %q, want openai", cfg.Proxy.ModelRouting.DefaultProvider)
	}
	if cfg.Proxy.ModelRouting.Prefixes["claude-"] != "anthropic" {
		t.Fatalf("claude prefix route = %q, want anthropic", cfg.Proxy.ModelRouting.Prefixes["claude-"])
	}
	if cfg.Proxy.ModelRouting.Prefixes["gemini-"] != "gemini" {
		t.Fatalf("gemini prefix route = %q, want gemini", cfg.Proxy.ModelRouting.Prefixes["gemini-"])
	}
	if cfg.Proxy.Gemini.BaseURL != "https://generativelanguage.googleapis.com/v1" {
		t.Fatalf("gemini.base_url = %q", cfg.Proxy.Gemini.BaseURL)
	}
	if cfg.Proxy.ModelRouting.Prefixes["deepseek-"] != "deepseek" {
		t.Fatalf("deepseek prefix route = %q, want deepseek", cfg.Proxy.ModelRouting.Prefixes["deepseek-"])
	}
}

func TestLoadModelRoutingConfig(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  openai:
    enabled: true
    base_url: "https://openai.test/v1"
    timeout: 10
  anthropic:
    enabled: true
    base_url: "https://anthropic.test/v1"
    timeout: 20
  gemini:
    enabled: true
    base_url: "https://gemini.test/v1"
    timeout: 30
  deepseek:
    enabled: true
    base_url: "https://deepseek.test/v1"
    timeout: 40
  model_routing:
    default_provider: "openai"
    models:
      claude-special: "anthropic"
    prefixes:
      gemini-pro: "gemini"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.DeepSeek.Timeout != 40 {
		t.Fatalf("deepseek timeout = %d, want 40", cfg.Proxy.DeepSeek.Timeout)
	}
	if cfg.Proxy.Gemini.BaseURL != "https://gemini.test/v1" {
		t.Fatalf("gemini base url = %q", cfg.Proxy.Gemini.BaseURL)
	}
	if cfg.Proxy.ModelRouting.Models["claude-special"] != "anthropic" {
		t.Fatalf("exact model route mismatch: %+v", cfg.Proxy.ModelRouting.Models)
	}
	if cfg.Proxy.ModelRouting.Prefixes["gemini-pro"] != "gemini" {
		t.Fatalf("prefix route mismatch: %+v", cfg.Proxy.ModelRouting.Prefixes)
	}
	if cfg.Proxy.ModelRouting.Prefixes["deepseek-"] != "deepseek" {
		t.Fatalf("expected default deepseek prefix to be retained")
	}
}

func TestLoadModelRoutingConfig_LegacyGoogleConfigFeedsGemini(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  google:
    enabled: true
    base_url: "https://legacy-google.test/v1"
    timeout: 31
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.Gemini.BaseURL != "https://legacy-google.test/v1" {
		t.Fatalf("gemini base url = %q, want legacy google value", cfg.Proxy.Gemini.BaseURL)
	}
	if cfg.Proxy.Gemini.Timeout != 31 {
		t.Fatalf("gemini timeout = %d, want 31", cfg.Proxy.Gemini.Timeout)
	}
}

func TestLoadModelRoutingConfig_DisabledProviderDoesNotGetDefaultPrefix(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  anthropic:
    enabled: false
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.Proxy.ModelRouting.Prefixes["claude-"]; ok {
		t.Fatalf("disabled anthropic provider should not receive default claude prefix: %+v", cfg.Proxy.ModelRouting.Prefixes)
	}
}

func TestLoadModelRoutingConfig_ExplicitEnabledProviderRequiresBaseURL(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  anthropic:
    enabled: true
    base_url: ""
    timeout: 30
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected explicit enabled provider with empty base_url to fail")
	}
}

func TestLoadModelRoutingConfig_ExplicitZeroTimeoutRejected(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  anthropic:
    enabled: false
    timeout: 0
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected explicit zero timeout to fail")
	}
}

func TestLoadModelRoutingConfig_DefaultProviderUsesEnabledProvider(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  openai:
    enabled: false
    base_url: "https://openai.test/v1"
    timeout: 30
  anthropic:
    enabled: true
    base_url: "https://anthropic.test/v1"
    timeout: 30
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Proxy.ModelRouting.DefaultProvider != "anthropic" {
		t.Fatalf("default provider = %q, want anthropic", cfg.Proxy.ModelRouting.DefaultProvider)
	}
	if _, ok := cfg.Proxy.ModelRouting.Prefixes["gpt-"]; ok {
		t.Fatalf("disabled openai provider should not receive default gpt prefix: %+v", cfg.Proxy.ModelRouting.Prefixes)
	}
}

func TestLoadModelRoutingConfig_InvalidRoutes(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "unknown provider",
			yaml: `proxy:
  model_routing:
    default_provider: "missing"
`,
		},
		{
			name: "disabled mapped provider",
			yaml: `proxy:
  anthropic:
    enabled: false
    base_url: "https://anthropic.test/v1"
    timeout: 30
  model_routing:
    default_provider: "openai"
    models:
      claude-3: "anthropic"
`,
		},
		{
			name: "empty prefix",
			yaml: `proxy:
  model_routing:
    default_provider: "openai"
    prefixes:
      "": "openai"
`,
		},
		{
			name: "legacy google route requires enabled gemini adapter",
			yaml: `proxy:
  gemini:
    enabled: false
    base_url: "https://gemini.test/v1"
    timeout: 30
  google:
    enabled: true
    base_url: "https://legacy-google.test/v1"
    timeout: 30
  model_routing:
    default_provider: "openai"
    prefixes:
      gemini-: "google"
`,
		},
		{
			name: "all providers disabled",
			yaml: `proxy:
  openai:
    enabled: false
    base_url: "https://openai.test/v1"
    timeout: 30
  anthropic:
    enabled: false
    base_url: "https://anthropic.test/v1"
    timeout: 30
  gemini:
    enabled: false
    base_url: "https://gemini.test/v1"
    timeout: 30
  deepseek:
    enabled: false
    base_url: "https://deepseek.test/v1"
    timeout: 30
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempConfig(t, tt.yaml)
			if _, err := Load(path); err == nil {
				t.Fatal("expected invalid model routing config to fail")
			}
		})
	}
}

func TestLoadBehaviorConfig_InvalidBackend(t *testing.T) {
	path := writeTempConfig(t, "proxy:\n  behavior:\n    token_store_backend: \"bad\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected invalid backend to fail")
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

func TestLoadConfigFromPathAllowsMissingDefaultFallbackOnly(t *testing.T) {
	t.Setenv("PORT", "6060")
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	exitCode := -1

	cfg := loadConfigFromPath(missing, true, func(code int) {
		exitCode = code
	})
	if cfg == nil {
		t.Fatal("expected default config for missing default path")
	}
	if exitCode != -1 {
		t.Fatalf("unexpected exit code %d", exitCode)
	}
	if cfg.Proxy.ModelRouting.DefaultProvider != "openai" {
		t.Fatalf("default_provider = %q, want openai", cfg.Proxy.ModelRouting.DefaultProvider)
	}
	if cfg.Server.Port != "6060" {
		t.Fatalf("port = %q, want env override 6060", cfg.Server.Port)
	}
}

func TestLoadConfigFromPathExitsOnInvalidConfig(t *testing.T) {
	path := writeTempConfig(t, `proxy:
  model_routing:
    default_provider: "missing"
`)
	exitCode := -1

	cfg := loadConfigFromPath(path, true, func(code int) {
		exitCode = code
	})
	if cfg != nil {
		t.Fatalf("expected nil config after fatal load failure")
	}
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
}

func TestLoadConfigFromPathExitsOnExplicitMissingConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	exitCode := -1

	cfg := loadConfigFromPath(missing, false, func(code int) {
		exitCode = code
	})
	if cfg != nil {
		t.Fatalf("expected nil config after explicit missing file")
	}
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
}

func TestLoadConfigFromPathAppliesEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "7070")
	path := writeTempConfig(t, "proxy: {}\n")

	cfg := loadConfigFromPath(path, false, func(code int) {
		t.Fatalf("unexpected exit %d", code)
	})
	if cfg.Server.Port != "7070" {
		t.Fatalf("port = %q, want 7070", cfg.Server.Port)
	}
}

func TestLoadConfigFromPathDoesNotFallbackOnParseError(t *testing.T) {
	path := writeTempConfig(t, "proxy:\n  model_routing: [")
	exitCode := -1

	cfg := loadConfigFromPath(path, true, func(code int) {
		exitCode = code
	})
	if cfg != nil {
		t.Fatalf("expected nil config after parse error")
	}
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "failed to parse config file") {
		t.Fatalf("expected parse error from Load, got %v", err)
	}
}
