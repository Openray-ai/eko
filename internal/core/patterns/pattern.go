package patterns

// Pattern represents a detection pattern definition
type Pattern struct {
	Name        string `yaml:"name" json:"name"`
	Regex       string `yaml:"regex" json:"regex"`
	Type        string `yaml:"type" json:"type"`
	Severity    string `yaml:"severity" json:"severity"`
	Description string `yaml:"description" json:"description"`
}

// PatternConfig holds pattern configuration from YAML
type PatternConfig struct {
	Patterns []Pattern `yaml:"patterns" json:"patterns"`
}

// PatternType constants
const (
	TypePII                  = "pii"
	TypeCredential           = "credential"
	TypeFinancial            = "financial"
	TypeBusinessIntelligence = "business_intelligence"
)

// Severity levels
const (
	SeverityBlock = "BLOCK"
	SeverityWarn  = "WARN"
	SeverityLog   = "LOG"
)
