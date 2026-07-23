package config

import (
	"eko/internal/helpers/logger"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// Config represents the application configuration
type CorsConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// Config represents the application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Patterns   PatternsConfig   `yaml:"patterns"`
	Alerts     AlertsConfig     `yaml:"alerts"`
	Compliance ComplianceConfig `yaml:"compliance"`
	Cors       CorsConfig       `yaml:"cors"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `yaml:"level"`       // debug, info, warn, error, fatal
	Format     string `yaml:"format"`      // text, json
	Colorize   bool   `yaml:"colorize"`    // Enable colored output
	OutputFile string `yaml:"output_file"` // Optional file output
}

// ProxyConfig holds proxy configuration for all providers
type ProxyConfig struct {
	OpenAI    ProviderConfig `yaml:"openai"`
	Anthropic ProviderConfig `yaml:"anthropic"`
	Google    ProviderConfig `yaml:"google"`
	Behavior  BehaviorConfig `yaml:"behavior"`
	Redis     RedisConfig    `yaml:"redis"`
	Crypto    CryptoConfig   `yaml:"crypto"`
	SLM       SLMConfig      `yaml:"slm"`
}

// SLMConfig configures the optional Small Language Model contextual detector.
// When Enabled is false (default), Ekō runs regex-only detection unchanged.
type SLMConfig struct {
	Enabled       bool                        `yaml:"enabled"`
	Endpoint      string                      `yaml:"endpoint"`        // default "http://slm-sidecar:8000"
	TimeoutMs     int                         `yaml:"timeout_ms"`      // default 800
	MaxInputBytes int                         `yaml:"max_input_bytes"` // default 16384; skip SLM for larger inputs
	Breaker       SLMBreakerConfig            `yaml:"breaker"`
	Labels        map[string]SLMLabelOverride `yaml:"labels"` // optional override of default mapping
}

// SLMBreakerConfig configures the in-process circuit breaker for SLM calls.
type SLMBreakerConfig struct {
	FailureThreshold int `yaml:"failure_threshold"` // default 5
	CooldownMs       int `yaml:"cooldown_ms"`       // default 30000
}

// SLMLabelOverride lets operators remap a single SLM span label.
// Empty fields fall back to the default mapping for that label.
type SLMLabelOverride struct {
	Type     string `yaml:"type"`
	Severity string `yaml:"severity"`
	Pattern  string `yaml:"pattern"`
}

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
	Timeout int    `yaml:"timeout"`
}

// BehaviorConfig defines how the proxy behaves on violations
type BehaviorConfig struct {
	OnViolation         string `yaml:"on_violation"` // block, sanitize, warn
	LogRequests         bool   `yaml:"log_requests"`
	AddViolationHeaders bool   `yaml:"add_violation_headers"`
	SanitizationMode    string `yaml:"sanitization_mode"` // redact, tokenize
	TokenStoreBackend   string `yaml:"token_store_backend"`
	TokenTTLms          int    `yaml:"token_ttl_ms"`
	MaxVaults           int    `yaml:"max_vaults"`
	MaxTokensPerVault   int    `yaml:"max_tokens_per_vault"`
	MaxBatchItems       int    `yaml:"max_batch_items"`
	MaxPromptBytes      int    `yaml:"max_prompt_bytes"`
	MaxBatchBytes       int    `yaml:"max_batch_bytes"`
	MaxBatchConcurrency int    `yaml:"max_batch_concurrency"`
}

type RedisConfig struct {
	Addr           string `yaml:"addr"`
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	DB             int    `yaml:"db"`
	KeyPrefix      string `yaml:"key_prefix"`
	MetaSuffix     string `yaml:"meta_suffix"`
	PayloadSuffix  string `yaml:"payload_suffix"`
	PoolSize       int    `yaml:"pool_size"`
	MinIdleConns   int    `yaml:"min_idle_conns"`
	DialTimeoutMs  int    `yaml:"dial_timeout_ms"`
	ReadTimeoutMs  int    `yaml:"read_timeout_ms"`
	WriteTimeoutMs int    `yaml:"write_timeout_ms"`
	MaxRetries     int    `yaml:"max_retries"`
}

type CryptoConfig struct {
	Provider       string             `yaml:"provider"`
	ActiveKeyID    string             `yaml:"active_key_id"`
	LocalMasterKey string             `yaml:"local_master_key"`
	FallbackKeys   []CryptoKeyConfig  `yaml:"fallback_keys"`
	VaultTransit   VaultTransitConfig `yaml:"vault_transit"`
}

type CryptoKeyConfig struct {
	KeyID     string `yaml:"key_id"`
	MasterKey string `yaml:"master_key"`
}

type VaultTransitConfig struct {
	Address       string `yaml:"address"`
	Token         string `yaml:"token"`
	Namespace     string `yaml:"namespace"`
	Mount         string `yaml:"mount"`
	KeyName       string `yaml:"key_name"`
	TimeoutMs     int    `yaml:"timeout_ms"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
}

// PatternsConfig holds pattern loading configuration
type PatternsConfig struct {
	ConfigFile        string `yaml:"config_file"`
	CustomPatternsDir string `yaml:"custom_patterns_dir"`
}

// AlertsConfig holds alerting configuration
type AlertsConfig struct {
	Webhooks []WebhookConfig `yaml:"webhooks"`
	Email    EmailConfig     `yaml:"email"`
}

// WebhookConfig holds webhook alert configuration
type WebhookConfig struct {
	URL      string   `yaml:"url"`
	Severity []string `yaml:"severity"`
}

// EmailConfig holds email alert configuration
type EmailConfig struct {
	SMTPHost string   `yaml:"smtp_host"`
	SMTPPort int      `yaml:"smtp_port"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
}

// ComplianceConfig holds compliance reporting configuration
type ComplianceConfig struct {
	EnableReporting bool   `yaml:"enable_reporting"`
	ReportDir       string `yaml:"report_dir"`
	RetentionDays   int    `yaml:"retention_days"`
}

// Load loads configuration from a YAML file
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		logger.Error("Failed to read config file", logger.Fields{
			"error":       err.Error(),
			"config_path": filename,
		})
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	applyBehaviorDefaults(&config.Proxy.Behavior)
	applyProxyDefaults(&config.Proxy)
	applySLMDefaults(&config.Proxy.SLM)
	if err := validateBehaviorConfig(config.Proxy.Behavior); err != nil {
		return nil, fmt.Errorf("invalid behavior config: %w", err)
	}
	if err := validateSLMConfig(config.Proxy.SLM); err != nil {
		return nil, fmt.Errorf("invalid slm config: %w", err)
	}

	return &config, nil
}

// Default returns a default configuration
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Port: "8080",
			Host: "0.0.0.0",
		},
		Logging: LoggingConfig{
			Level:    "debug",
			Format:   "text",
			Colorize: true,
		},
		Proxy: ProxyConfig{
			OpenAI: ProviderConfig{
				Enabled: true,
				BaseURL: "https://api.openai.com/v1",
				Timeout: 30,
			},
			Anthropic: ProviderConfig{
				Enabled: true,
				BaseURL: "https://api.anthropic.com/v1",
				Timeout: 30,
			},
			Google: ProviderConfig{
				Enabled: true,
				BaseURL: "https://generativelanguage.googleapis.com/v1",
				Timeout: 30,
			},
			Behavior: BehaviorConfig{
				OnViolation:         "sanitize",
				LogRequests:         true,
				AddViolationHeaders: true,
				SanitizationMode:    "redact",
				TokenStoreBackend:   "memory",
				TokenTTLms:          30000,
				MaxVaults:           10000,
				MaxTokensPerVault:   100000,
				MaxBatchItems:       100,
				MaxPromptBytes:      65536,
				MaxBatchBytes:       1048576,
				MaxBatchConcurrency: 1,
			},
			Redis: RedisConfig{
				KeyPrefix:      "eko:",
				MetaSuffix:     "vault_meta:",
				PayloadSuffix:  "vault_payload:",
				PoolSize:       10,
				MinIdleConns:   2,
				DialTimeoutMs:  100,
				ReadTimeoutMs:  100,
				WriteTimeoutMs: 100,
				MaxRetries:     3,
			},
			Crypto: CryptoConfig{
				Provider:    "local",
				ActiveKeyID: "local-dev",
				VaultTransit: VaultTransitConfig{
					Mount:     "transit",
					KeyName:   "eko-session",
					TimeoutMs: 2000,
				},
			},
		},
		Patterns: PatternsConfig{
			ConfigFile:        "./patterns/default.yaml",
			CustomPatternsDir: "./patterns/custom",
		},
	}
}

func applyBehaviorDefaults(cfg *BehaviorConfig) {
	if cfg.SanitizationMode == "" {
		cfg.SanitizationMode = "redact"
	}
	if cfg.TokenTTLms == 0 {
		cfg.TokenTTLms = 30000
	}
	if cfg.TokenStoreBackend == "" {
		cfg.TokenStoreBackend = "memory"
	}
	if cfg.MaxVaults == 0 {
		cfg.MaxVaults = 10000
	}
	if cfg.MaxTokensPerVault == 0 {
		cfg.MaxTokensPerVault = 100000
	}
	if cfg.MaxBatchItems == 0 {
		cfg.MaxBatchItems = 100
	}
	if cfg.MaxPromptBytes == 0 {
		cfg.MaxPromptBytes = 65536
	}
	if cfg.MaxBatchBytes == 0 {
		cfg.MaxBatchBytes = 1048576
	}
	if cfg.MaxBatchConcurrency == 0 {
		cfg.MaxBatchConcurrency = 1
	}
}

func applyProxyDefaults(cfg *ProxyConfig) {
	if cfg.Redis.KeyPrefix == "" {
		cfg.Redis.KeyPrefix = "eko:"
	}
	if cfg.Redis.MetaSuffix == "" {
		cfg.Redis.MetaSuffix = "vault_meta:"
	}
	if cfg.Redis.PayloadSuffix == "" {
		cfg.Redis.PayloadSuffix = "vault_payload:"
	}
	if cfg.Crypto.Provider == "" {
		cfg.Crypto.Provider = "local"
	}
	if cfg.Crypto.ActiveKeyID == "" {
		cfg.Crypto.ActiveKeyID = "local-dev"
	}
	if cfg.Crypto.VaultTransit.Mount == "" {
		cfg.Crypto.VaultTransit.Mount = "transit"
	}
	if cfg.Crypto.VaultTransit.KeyName == "" {
		cfg.Crypto.VaultTransit.KeyName = "eko-session"
	}
	if cfg.Crypto.VaultTransit.TimeoutMs == 0 {
		cfg.Crypto.VaultTransit.TimeoutMs = 2000
	}
}

func applySLMDefaults(cfg *SLMConfig) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://slm-sidecar:8000"
	}
	if cfg.TimeoutMs == 0 {
		cfg.TimeoutMs = 800
	}
	if cfg.MaxInputBytes == 0 {
		cfg.MaxInputBytes = 16384
	}
	if cfg.Breaker.FailureThreshold == 0 {
		cfg.Breaker.FailureThreshold = 5
	}
	if cfg.Breaker.CooldownMs == 0 {
		cfg.Breaker.CooldownMs = 30000
	}
}

func validateSLMConfig(cfg SLMConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Endpoint == "" {
		return fmt.Errorf("endpoint is required when slm.enabled is true")
	}
	if cfg.TimeoutMs <= 0 {
		return fmt.Errorf("timeout_ms must be > 0")
	}
	if cfg.MaxInputBytes <= 0 {
		return fmt.Errorf("max_input_bytes must be > 0")
	}
	if cfg.Breaker.FailureThreshold <= 0 {
		return fmt.Errorf("breaker.failure_threshold must be > 0")
	}
	if cfg.Breaker.CooldownMs <= 0 {
		return fmt.Errorf("breaker.cooldown_ms must be > 0")
	}
	for label, ov := range cfg.Labels {
		if ov.Severity != "" && ov.Severity != "BLOCK" && ov.Severity != "WARN" && ov.Severity != "LOG" {
			return fmt.Errorf("labels[%q].severity must be BLOCK, WARN, or LOG", label)
		}
	}
	return nil
}

func validateBehaviorConfig(cfg BehaviorConfig) error {
	if cfg.SanitizationMode != "redact" && cfg.SanitizationMode != "tokenize" {
		return fmt.Errorf("sanitization_mode must be \"redact\" or \"tokenize\"")
	}
	if cfg.TokenStoreBackend != "memory" && cfg.TokenStoreBackend != "redis" {
		return fmt.Errorf("token_store_backend must be \"memory\" or \"redis\"")
	}
	if cfg.TokenTTLms <= 0 {
		return fmt.Errorf("token_ttl_ms must be > 0")
	}
	if cfg.MaxVaults <= 0 {
		return fmt.Errorf("max_vaults must be > 0")
	}
	if cfg.MaxTokensPerVault <= 0 {
		return fmt.Errorf("max_tokens_per_vault must be > 0")
	}
	if cfg.MaxBatchItems <= 0 {
		return fmt.Errorf("max_batch_items must be > 0")
	}
	if cfg.MaxPromptBytes <= 0 {
		return fmt.Errorf("max_prompt_bytes must be > 0")
	}
	if cfg.MaxBatchBytes <= 0 {
		return fmt.Errorf("max_batch_bytes must be > 0")
	}
	if cfg.MaxBatchConcurrency <= 0 {
		return fmt.Errorf("max_batch_concurrency must be > 0")
	}
	return nil
}

func LoadConfig() *Config {
	configPath := os.Getenv("EKO_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfg, err := Load(configPath)
	if err != nil {
		// Use fmt here since logger isn't initialized yet
		fmt.Printf("Failed to load config from %s: %v\n", configPath, err)
		fmt.Println("Using default configuration")
		cfg = Default()
	}

	applyEnvOverrides(cfg)
	return cfg
}

// applyEnvOverrides lets PaaS environments (Fly, Railway, Cloud Run) inject
// values that vary per deployment without templating the YAML. Only non-empty
// env vars override; empty vars leave the YAML/default value in place.
//
// Note: CORS origins are overridden in middleware.CORS() because that path
// loads its own config file (configs/cors.yml). See EKO_ALLOWED_ORIGINS in
// internal/api/middleware/cors.go.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("PORT"); v != "" {
		cfg.Server.Port = v
	}
	if v := os.Getenv("EKO_SLM_ENDPOINT"); v != "" {
		cfg.Proxy.SLM.Endpoint = v
	}
}

func InitializeLogger(cfg *Config) {
	// Parse log level
	level, err := logger.ParseLevel(cfg.Logging.Level)
	if err != nil {
		fmt.Printf("Invalid log level '%s', using INFO: %v\n", cfg.Logging.Level, err)
		level = logger.InfoLevel
	}

	// Determine output
	output := os.Stdout
	if cfg.Logging.OutputFile != "" {
		file, err := os.OpenFile(cfg.Logging.OutputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("Failed to open log file %s: %v\n", cfg.Logging.OutputFile, err)
			fmt.Println("Falling back to stdout")
		} else {
			output = file
		}
	}

	// Initialize logger
	logger.Initialize(logger.Config{
		Level:      level,
		Output:     output,
		JSONFormat: cfg.Logging.Format == "json",
		Colorize:   cfg.Logging.Colorize && cfg.Logging.Format != "json",
	})
}
