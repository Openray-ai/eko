package patterns

import "regexp"

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

// CompiledPattern represents a compiled regex pattern with metadata
type CompiledPattern struct {
	Name        string
	Regex       *regexp.Regexp
	Type        string
	Severity    string
	Description string
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

// PatternName constants for pattern identifiers used in code
const (
	PatternEmail                       = "email"
	PatternNigerianBVN                 = "nigerian_bvn"
	PatternNigerianPhone               = "nigerian_phone"
	PatternNigerianAccount             = "nigerian_account"
	PatternKenyanPhone                 = "kenyan_phone"
	PatternMpesaCode                   = "mpesa_code"
	PatternSouthAfricanID              = "south_african_id"
	PatternSouthAfricanPhone           = "south_african_phone"
	PatternGhanaianPhone               = "ghanaian_phone"
	PatternCreditCard                  = "credit_card"
	PatternIBAN                        = "iban"
	PatternSwiftCode                   = "swift_code"
	PatternOpenAIAPIKey                = "openai_api_key"
	PatternAnthropicAPIKey             = "anthropic_api_key"
	PatternGoogleAPIKey                = "google_api_key"
	PatternAWSAccessKey                = "aws_access_key"
	PatternAWSAccessKeyID              = "aws_access_key_id"
	PatternAWSSecretAccessKey          = "aws_secret_access_key"
	PatternAWSSessionToken             = "aws_session_token"
	PatternGenericSecretAssignment     = "generic_secret_assignment"
	PatternGenericAPIKeyAssignment     = "generic_api_key_assignment"
	PatternGenericTokenAssignment      = "generic_token_assignment"
	PatternGenericPrivateKeyAssignment = "generic_private_key_assignment"
	PatternPostgresConn                = "postgres_connection"
	PatternMongoDBConn                 = "mongodb_connection"
	PatternMySQLConn                   = "mysql_connection"
	PatternJWTToken                    = "jwt_token"
	PatternSSHPrivateKey               = "ssh_private_key"
	PatternPasswordVar                 = "password_var"
	PatternDateOfBirth                 = "date_of_birth"
	PatternPostalCode                  = "postal_code"

	// PhoneSuffix is the common suffix for phone-type patterns, used for
	// dynamic generator selection in the tokenizer.
	PhoneSuffix = "_phone"
)
