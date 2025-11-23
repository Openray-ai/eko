package sanitizer

import (
	"eko/internal/core/detector"
	"eko/internal/helpers/logger"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Sanitizer handles redaction and replacement of sensitive data
type Sanitizer struct {
	detector *detector.Detector
}

// Result contains the sanitized output and violations found
type Result struct {
	OriginalPrompt   string               `json:"original_prompt,omitempty"`
	SanitizedPrompt  string               `json:"sanitized_prompt"`
	Violations       []detector.Violation `json:"violations"`
	Safe             bool                 `json:"safe"`
	ProcessingTimeMs float64              `json:"processing_time_ms"`
	RedactedCount    int                  `json:"redacted_count"`
}

// New creates a new Sanitizer instance
func New(det *detector.Detector) *Sanitizer {
	return &Sanitizer{
		detector: det,
	}
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
		if v.Severity == "BLOCK" || v.Severity == "WARN" {
			// Safety check: ensure position is valid
			if v.Position >= 0 && v.End <= len(result) && v.Position < v.End {
				result = result[:v.Position] + replacement + result[v.End:]
			}
		}
		// LOG severity: don't redact, just log
	}

	return result
}

// getReplacementText returns the replacement text for a violation based on severity
func (s *Sanitizer) getReplacementText(v detector.Violation) string {
	switch v.Severity {
	case "BLOCK":
		// For BLOCK severity, use specific redaction based on pattern type
		return s.getRedactionLabel(v.Pattern, v.Type)
	case "WARN":
		// For WARN severity, use a generic warning label
		return fmt.Sprintf("[WARNING_%s]", strings.ToUpper(v.Pattern))
	case "LOG":
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
		"openai_api_key":      "[REDACTED_API_KEY]",
		"anthropic_api_key":   "[REDACTED_API_KEY]",
		"google_api_key":      "[REDACTED_API_KEY]",
		"aws_access_key":      "[REDACTED_AWS_KEY]",
		"postgres_connection": "[REDACTED_DB_CONNECTION]",
		"mongodb_connection":  "[REDACTED_DB_CONNECTION]",
		"mysql_connection":    "[REDACTED_DB_CONNECTION]",
		"jwt_token":           "[REDACTED_TOKEN]",
		"email":               "[REDACTED_EMAIL]",
		"nigerian_bvn":        "[REDACTED_BVN]",
		"nigerian_phone":      "[REDACTED_PHONE]",
		"nigerian_account":    "[REDACTED_ACCOUNT]",
		"kenyan_phone":        "[REDACTED_PHONE]",
		"mpesa_code":          "[REDACTED_MPESA]",
		"south_african_id":    "[REDACTED_ID]",
		"south_african_phone": "[REDACTED_PHONE]",
		"ghanaian_phone":      "[REDACTED_PHONE]",
		"credit_card":         "[REDACTED_CARD]",
		"iban":                "[REDACTED_IBAN]",
		"swift_code":          "[REDACTED_SWIFT]",
		"ssh_private_key":     "[REDACTED_PRIVATE_KEY]",
		"password_var":        "[REDACTED_PASSWORD]",
	}

	// Return specific label if available
	if label, exists := labels[patternName]; exists {
		return label
	}

	// Fallback to generic label based on type
	switch patternType {
	case "pii":
		return "[REDACTED_PII]"
	case "credential":
		return "[REDACTED_CREDENTIAL]"
	case "financial":
		return "[REDACTED_FINANCIAL]"
	case "business_intelligence":
		return "[REDACTED_BUSINESS_DATA]"
	default:
		return "[REDACTED]"
	}
}

// countRedacted counts how many violations were actually redacted (BLOCK + WARN)
func (s *Sanitizer) countRedacted(violations []detector.Violation) int {
	count := 0
	for _, v := range violations {
		if v.Severity == "BLOCK" || v.Severity == "WARN" {
			count++
		}
	}
	return count
}
