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
	TokenTTLms          int    `yaml:"token_ttl_ms"`
	MaxVaults           int    `yaml:"max_vaults"`
	MaxTokensPerVault   int    `yaml:"max_tokens_per_vault"`
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
	if err := validateBehaviorConfig(config.Proxy.Behavior); err != nil {
		return nil, fmt.Errorf("invalid behavior config: %w", err)
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
				TokenTTLms:          30000,
				MaxVaults:           10000,
				MaxTokensPerVault:   100000,
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
	if cfg.MaxVaults == 0 {
		cfg.MaxVaults = 10000
	}
	if cfg.MaxTokensPerVault == 0 {
		cfg.MaxTokensPerVault = 100000
	}
}

func validateBehaviorConfig(cfg BehaviorConfig) error {
	if cfg.SanitizationMode != "redact" && cfg.SanitizationMode != "tokenize" {
		return fmt.Errorf("sanitization_mode must be \"redact\" or \"tokenize\"")
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
		return Default()
	}

	return cfg
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
