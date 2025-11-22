package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// Config represents the application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Patterns   PatternsConfig   `yaml:"patterns"`
	Alerts     AlertsConfig     `yaml:"alerts"`
	Compliance ComplianceConfig `yaml:"compliance"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"`
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
	OnViolation          string `yaml:"on_violation"` // block, sanitize, warn
	LogRequests          bool   `yaml:"log_requests"`
	AddViolationHeaders  bool   `yaml:"add_violation_headers"`
}

// PatternsConfig holds pattern loading configuration
type PatternsConfig struct {
	ConfigFile         string `yaml:"config_file"`
	CustomPatternsDir  string `yaml:"custom_patterns_dir"`
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
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
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
			},
		},
		Patterns: PatternsConfig{
			ConfigFile:        "./patterns/default.yaml",
			CustomPatternsDir: "./patterns/custom",
		},
	}
}
