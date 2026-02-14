package sanitizer

import (
	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/tokenizer"
	"eko/internal/helpers/logger"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Sanitizer handles redaction and replacement of sensitive data
type Sanitizer struct {
	detector     *detector.Detector
	tokenizer    *tokenizer.Tokenizer
	vaultManager *tokenizer.VaultManager
	mode         string
}

// Result contains the sanitized output and violations found
type Result struct {
	OriginalPrompt   string               `json:"original_prompt,omitempty"`
	SanitizedPrompt  string               `json:"sanitized_prompt"`
	Violations       []detector.Violation `json:"violations"`
	Safe             bool                 `json:"safe"`
	ProcessingTimeMs float64              `json:"processing_time_ms"`
	RedactedCount    int                  `json:"redacted_count"`
	TokenizedCount   int                  `json:"tokenized_count"`
	SessionID        string               `json:"session_id,omitempty"`
}

// New creates a new Sanitizer instance
func New(det *detector.Detector) *Sanitizer {
	return &Sanitizer{
		detector: det,
		mode:     "redact",
	}
}

// NewWithTokenizer creates a new Sanitizer instance with tokenization support
func NewWithTokenizer(det *detector.Detector, tok *tokenizer.Tokenizer, vm *tokenizer.VaultManager, mode string) *Sanitizer {
	if mode == "" {
		mode = "redact"
	}

	return &Sanitizer{
		detector:     det,
		tokenizer:    tok,
		vaultManager: vm,
		mode:         mode,
	}
}

// SanitizationMode returns the active sanitization mode.
func (s *Sanitizer) SanitizationMode() string {
	if s.mode == "" {
		return "redact"
	}
	return s.mode
}

// Sanitize detects and redacts sensitive data from input
func (s *Sanitizer) Sanitize(input string) (*Result, error) {
	startTime := time.Now()

	// Detect violations
	violations, err := s.detector.Detect(input)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}

	// If no violations found, return original input
	if len(violations) == 0 {
		return &Result{
			OriginalPrompt:   input,
			SanitizedPrompt:  input,
			Violations:       violations,
			Safe:             true,
			ProcessingTimeMs: float64(time.Since(startTime).Microseconds()) / 1000.0,
			RedactedCount:    0,
		}, nil
	}

	// Apply redaction strategies based on severity
	sanitized := s.redactViolations(input, violations)

	// Calculate processing time
	processingTime := float64(time.Since(startTime).Microseconds()) / 1000.0

	logger.Info("Sanitization completed", logger.Fields{
		"violations":       len(violations),
		"redacted":         s.countRedacted(violations),
		"processing_ms":    processingTime,
		"input_length":     len(input),
		"sanitized_length": len(sanitized),
	})

	return &Result{
		OriginalPrompt:   input,
		SanitizedPrompt:  sanitized,
		Violations:       violations,
		Safe:             false,
		ProcessingTimeMs: processingTime,
		RedactedCount:    s.countRedacted(violations),
		TokenizedCount:   0,
	}, nil
}

// SanitizeWithSession detects and sanitizes sensitive data from input with session awareness
func (s *Sanitizer) SanitizeWithSession(input, sessionID string) (*Result, error) {
	startTime := time.Now()

	violations, err := s.detector.Detect(input)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}

	if len(violations) == 0 {
		return &Result{
			OriginalPrompt:   input,
			SanitizedPrompt:  input,
			Violations:       violations,
			Safe:             true,
			ProcessingTimeMs: float64(time.Since(startTime).Microseconds()) / 1000.0,
			RedactedCount:    0,
			TokenizedCount:   0,
			SessionID:        sessionID,
		}, nil
	}

	mode := s.mode
	if mode == "" {
		mode = "redact"
	}

	var sanitized string
	tokenizedCount := 0

	switch mode {
	case "tokenize":
		if s.tokenizer == nil || s.vaultManager == nil {
			return nil, fmt.Errorf("tokenization requires tokenizer and vault manager")
		}
		var err error
		sanitized, tokenizedCount, err = s.tokenizeViolationsWithCount(input, violations, sessionID)
		if err != nil {
			return nil, err
		}
	default:
		sanitized = s.redactViolations(input, violations)
	}

	processingTime := float64(time.Since(startTime).Microseconds()) / 1000.0

	logger.Info("Sanitization completed", logger.Fields{
		"violations":       len(violations),
		"redacted":         s.countRedacted(violations),
		"tokenized":        tokenizedCount,
		"processing_ms":    processingTime,
		"input_length":     len(input),
		"sanitized_length": len(sanitized),
	})

	return &Result{
		OriginalPrompt:   input,
		SanitizedPrompt:  sanitized,
		Violations:       violations,
		Safe:             false,
		ProcessingTimeMs: processingTime,
		RedactedCount:    s.countRedacted(violations),
		TokenizedCount:   tokenizedCount,
		SessionID:        sessionID,
	}, nil
}

// redactViolations applies redaction strategies to the input based on violation severity
func (s *Sanitizer) redactViolations(input string, violations []detector.Violation) string {
	if len(violations) == 0 {
		return input
	}

	// Sort violations by position (descending) to avoid offset issues during replacement
	sorted := make([]detector.Violation, len(violations))
	copy(sorted, violations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Position > sorted[j].Position
	})

	// Build sanitized string by replacing violations from end to start
	result := input
	for _, v := range sorted {
		replacement := s.getReplacementText(v)

		// Only replace BLOCK and WARN severity violations
		if v.Severity == patterns.SeverityBlock || v.Severity == patterns.SeverityWarn {
			// Safety check: ensure position is valid
			if v.Position >= 0 && v.End <= len(result) && v.Position < v.End {
				result = result[:v.Position] + replacement + result[v.End:]
			}
		}
		// LOG severity: don't redact, just log
	}

	return result
}

func (s *Sanitizer) tokenizeViolationsWithCount(input string, violations []detector.Violation, sessionID string) (string, int, error) {
	if len(violations) == 0 {
		return input, 0, nil
	}

	vault, err := s.vaultManager.GetOrCreate(sessionID)
	if err != nil {
		return "", 0, err
	}

	sorted := make([]detector.Violation, len(violations))
	copy(sorted, violations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Position > sorted[j].Position
	})

	result := input
	tokenizedCount := 0

	for _, v := range sorted {
		if v.Severity != patterns.SeverityBlock && v.Severity != patterns.SeverityWarn {
			continue
		}

		var replacement string
		switch v.Type {
		case patterns.TypeCredential:
			replacement = s.getRedactionLabel(v.Pattern, v.Type)
		default:
			token, err := s.tokenizer.Generate(v, vault)
			if err != nil {
				return "", 0, err
			}
			replacement = token
			tokenizedCount++
		}

		if v.Position >= 0 && v.End <= len(result) && v.Position < v.End {
			result = result[:v.Position] + replacement + result[v.End:]
		}
	}

	return result, tokenizedCount, nil
}

// getReplacementText returns the replacement text for a violation based on severity
func (s *Sanitizer) getReplacementText(v detector.Violation) string {
	switch v.Severity {
	case patterns.SeverityBlock:
		// For BLOCK severity, use specific redaction based on pattern type
		return s.getRedactionLabel(v.Pattern, v.Type)
	case patterns.SeverityWarn:
		// For WARN severity, use a generic warning label
		return fmt.Sprintf("[WARNING_%s]", strings.ToUpper(v.Pattern))
	case patterns.SeverityLog:
		// For LOG severity, don't redact (return original matched text)
		return v.Matched
	default:
		return "[REDACTED]"
	}
}

// getRedactionLabel returns a specific redaction label based on pattern name and type
func (s *Sanitizer) getRedactionLabel(patternName, patternType string) string {
	// Map common pattern names to user-friendly labels
	labels := map[string]string{
		patterns.PatternOpenAIAPIKey:      "[REDACTED_API_KEY]",
		patterns.PatternAnthropicAPIKey:   "[REDACTED_API_KEY]",
		patterns.PatternGoogleAPIKey:      "[REDACTED_API_KEY]",
		patterns.PatternAWSAccessKey:      "[REDACTED_AWS_KEY]",
		patterns.PatternPostgresConn:      "[REDACTED_DB_CONNECTION]",
		patterns.PatternMongoDBConn:       "[REDACTED_DB_CONNECTION]",
		patterns.PatternMySQLConn:         "[REDACTED_DB_CONNECTION]",
		patterns.PatternJWTToken:          "[REDACTED_TOKEN]",
		patterns.PatternEmail:             "[REDACTED_EMAIL]",
		patterns.PatternNigerianBVN:       "[REDACTED_BVN]",
		patterns.PatternNigerianPhone:     "[REDACTED_PHONE]",
		patterns.PatternNigerianAccount:   "[REDACTED_ACCOUNT]",
		patterns.PatternKenyanPhone:       "[REDACTED_PHONE]",
		patterns.PatternMpesaCode:         "[REDACTED_MPESA]",
		patterns.PatternSouthAfricanID:    "[REDACTED_ID]",
		patterns.PatternSouthAfricanPhone: "[REDACTED_PHONE]",
		patterns.PatternGhanaianPhone:     "[REDACTED_PHONE]",
		patterns.PatternCreditCard:        "[REDACTED_CARD]",
		patterns.PatternIBAN:              "[REDACTED_IBAN]",
		patterns.PatternSwiftCode:         "[REDACTED_SWIFT]",
		patterns.PatternSSHPrivateKey:     "[REDACTED_PRIVATE_KEY]",
		patterns.PatternPasswordVar:       "[REDACTED_PASSWORD]",
		patterns.PatternDateOfBirth:       "[REDACTED_DOB]",
		patterns.PatternPostalCode:        "[REDACTED_POSTAL_CODE]",
	}

	// Return specific label if available
	if label, exists := labels[patternName]; exists {
		return label
	}

	// Fallback to generic label based on type
	switch patternType {
	case patterns.TypePII:
		return "[REDACTED_PII]"
	case patterns.TypeCredential:
		return "[REDACTED_CREDENTIAL]"
	case patterns.TypeFinancial:
		return "[REDACTED_FINANCIAL]"
	case patterns.TypeBusinessIntelligence:
		return "[REDACTED_BUSINESS_DATA]"
	default:
		return "[REDACTED]"
	}
}

// countRedacted counts how many violations were actually redacted (BLOCK + WARN)
func (s *Sanitizer) countRedacted(violations []detector.Violation) int {
	count := 0
	for _, v := range violations {
		if v.Severity == patterns.SeverityBlock || v.Severity == patterns.SeverityWarn {
			count++
		}
	}
	return count
}
