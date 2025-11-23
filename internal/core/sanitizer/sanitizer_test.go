package sanitizer

import (
	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"regexp"
	"strings"
	"testing"
)

func TestSanitizer_New(t *testing.T) {
	det := detector.New()
	s := New(det)

	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.detector == nil {
		t.Error("detector not initialized")
	}
}

func TestSanitizer_Sanitize_NoViolations(t *testing.T) {
	det := detector.New()
	s := New(det)

	input := "This is a safe string with no sensitive data"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Safe {
		t.Error("expected Safe to be true")
	}

	if result.SanitizedPrompt != input {
		t.Error("sanitized prompt should match input when no violations")
	}

	if len(result.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(result.Violations))
	}

	if result.RedactedCount != 0 {
		t.Errorf("expected redacted count to be 0, got %d", result.RedactedCount)
	}
}

func TestSanitizer_Sanitize_EmailRedaction(t *testing.T) {
	det := detector.New()
	emailPattern := &patterns.CompiledPattern{
		Name:        "email",
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        "pii",
		Severity:    "BLOCK",
		Description: "Email address",
	}
	det.LoadPattern(emailPattern)

	s := New(det)

	input := "My email is john@example.com"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Safe {
		t.Error("expected Safe to be false")
	}

	if len(result.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(result.Violations))
	}

	// Verify email was redacted
	if strings.Contains(result.SanitizedPrompt, "john@example.com") {
		t.Error("email should have been redacted")
	}

	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_EMAIL]") {
		t.Errorf("expected sanitized prompt to contain redaction marker, got: %s", result.SanitizedPrompt)
	}

	if result.RedactedCount != 1 {
		t.Errorf("expected redacted count to be 1, got %d", result.RedactedCount)
	}
}

func TestSanitizer_Sanitize_MultipleViolations(t *testing.T) {
	det := detector.New()

	patterns := []*patterns.CompiledPattern{
		{
			Name:        "email",
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        "pii",
			Severity:    "BLOCK",
			Description: "Email address",
		},
		{
			Name:        "openai_api_key",
			Regex:       regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),
			Type:        "credential",
			Severity:    "BLOCK",
			Description: "OpenAI API key",
		},
	}
	det.LoadPatterns(patterns)

	s := New(det)

	// sk- followed by exactly 48 alphanumeric characters
	input := "Contact john@example.com with API key sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Safe {
		t.Error("expected Safe to be false")
	}

	if len(result.Violations) < 2 {
		t.Fatalf("expected at least 2 violations, got %d", len(result.Violations))
	}

	// Verify both were redacted
	if strings.Contains(result.SanitizedPrompt, "john@example.com") {
		t.Error("email should have been redacted")
	}

	if strings.Contains(result.SanitizedPrompt, "sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd") {
		t.Error("API key should have been redacted")
	}
}

func TestSanitizer_Sanitize_SeverityLevels(t *testing.T) {
	tests := []struct {
		name           string
		severity       string
		shouldRedact   bool
		expectedMarker string
	}{
		{
			name:           "BLOCK severity",
			severity:       "BLOCK",
			shouldRedact:   true,
			expectedMarker: "[REDACTED_EMAIL]",
		},
		{
			name:           "WARN severity",
			severity:       "WARN",
			shouldRedact:   true,
			expectedMarker: "[WARNING_EMAIL]",
		},
		{
			name:           "LOG severity",
			severity:       "LOG",
			shouldRedact:   false,
			expectedMarker: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			det := detector.New()
			emailPattern := &patterns.CompiledPattern{
				Name:        "email",
				Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
				Type:        "pii",
				Severity:    tt.severity,
				Description: "Email address",
			}
			det.LoadPattern(emailPattern)

			s := New(det)

			input := "Contact john@example.com"
			result, err := s.Sanitize(input)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.shouldRedact {
				if strings.Contains(result.SanitizedPrompt, "john@example.com") {
					t.Error("email should have been redacted")
				}
				if tt.expectedMarker != "" && !strings.Contains(result.SanitizedPrompt, tt.expectedMarker) {
					t.Errorf("expected marker '%s' in sanitized output, got: %s", tt.expectedMarker, result.SanitizedPrompt)
				}
			} else {
				// LOG severity shouldn't redact
				if !strings.Contains(result.SanitizedPrompt, "john@example.com") {
					t.Error("LOG severity should not redact")
				}
			}
		})
	}
}

func TestSanitizer_Sanitize_PreservesStructure(t *testing.T) {
	det := detector.New()
	emailPattern := &patterns.CompiledPattern{
		Name:        "email",
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        "pii",
		Severity:    "BLOCK",
		Description: "Email address",
	}
	det.LoadPattern(emailPattern)

	s := New(det)

	input := "Please contact:\n1. john@example.com\n2. jane@example.org\nThank you!"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify newlines are preserved
	if strings.Count(result.SanitizedPrompt, "\n") != strings.Count(input, "\n") {
		t.Error("sanitization should preserve newlines")
	}

	// Verify structure words are still there
	expectedWords := []string{"Please", "contact:", "1.", "2.", "Thank", "you!"}
	for _, word := range expectedWords {
		if !strings.Contains(result.SanitizedPrompt, word) {
			t.Errorf("expected word '%s' to be preserved", word)
		}
	}
}

func TestSanitizer_Sanitize_ProcessingTime(t *testing.T) {
	det := detector.New()
	s := New(det)

	input := "This is a test string"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ProcessingTimeMs <= 0 {
		t.Error("processing time should be greater than 0")
	}

	// Should be reasonably fast (< 100ms for simple input)
	if result.ProcessingTimeMs > 100 {
		t.Errorf("processing time seems too high: %.2fms", result.ProcessingTimeMs)
	}
}

func TestSanitizer_Sanitize_EmptyInput(t *testing.T) {
	det := detector.New()
	s := New(det)

	result, err := s.Sanitize("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Safe {
		t.Error("empty input should be safe")
	}

	if result.SanitizedPrompt != "" {
		t.Error("sanitized output should be empty for empty input")
	}
}

func TestSanitizer_GetRedactionLabel(t *testing.T) {
	s := New(detector.New())

	tests := []struct {
		patternName string
		patternType string
		expected    string
	}{
		{"email", "pii", "[REDACTED_EMAIL]"},
		{"openai_api_key", "credential", "[REDACTED_API_KEY]"},
		{"nigerian_bvn", "pii", "[REDACTED_BVN]"},
		{"credit_card", "financial", "[REDACTED_CARD]"},
		{"unknown_pattern", "pii", "[REDACTED_PII]"},
		{"unknown_pattern", "credential", "[REDACTED_CREDENTIAL]"},
		{"unknown_pattern", "financial", "[REDACTED_FINANCIAL]"},
		{"unknown_pattern", "unknown_type", "[REDACTED]"},
	}

	for _, tt := range tests {
		t.Run(tt.patternName, func(t *testing.T) {
			result := s.getRedactionLabel(tt.patternName, tt.patternType)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestSanitizer_CountRedacted(t *testing.T) {
	s := New(detector.New())

	violations := []detector.Violation{
		{Severity: "BLOCK"},
		{Severity: "WARN"},
		{Severity: "LOG"},
		{Severity: "BLOCK"},
	}

	count := s.countRedacted(violations)

	// BLOCK and WARN are redacted, LOG is not
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestSanitizer_Sanitize_RealWorldExample(t *testing.T) {
	det := detector.New()

	patterns := []*patterns.CompiledPattern{
		{
			Name:        "email",
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        "pii",
			Severity:    "WARN",
			Description: "Email address",
		},
		{
			Name:        "postgres_connection",
			Regex:       regexp.MustCompile(`postgres://[^\s]+`),
			Type:        "credential",
			Severity:    "BLOCK",
			Description: "PostgreSQL connection string",
		},
		{
			Name:        "nigerian_bvn",
			Regex:       regexp.MustCompile(`\b\d{11}\b`),
			Type:        "pii",
			Severity:    "BLOCK",
			Description: "Nigerian BVN",
		},
	}
	det.LoadPatterns(patterns)

	s := New(det)

	// Example from README
	input := "Debug this error: postgres://admin:SecureP@ss123@prod-db.company.com and my BVN is 12345678901"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Safe {
		t.Error("expected Safe to be false")
	}

	// Verify sensitive data was redacted
	if strings.Contains(result.SanitizedPrompt, "SecureP@ss123") {
		t.Error("password should have been redacted")
	}

	if strings.Contains(result.SanitizedPrompt, "12345678901") {
		t.Error("BVN should have been redacted")
	}

	if len(result.Violations) < 2 {
		t.Errorf("expected at least 2 violations, got %d", len(result.Violations))
	}

	t.Logf("Original: %s", input)
	t.Logf("Sanitized: %s", result.SanitizedPrompt)
	t.Logf("Violations: %d", len(result.Violations))
}
