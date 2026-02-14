package sanitizer

import (
	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/tokenizer"
	"regexp"
	"strings"
	"testing"
	"time"
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
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
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

	testPatterns := []*patterns.CompiledPattern{
		{
			Name:        patterns.PatternEmail,
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Email address",
		},
		{
			Name:        patterns.PatternOpenAIAPIKey,
			Regex:       regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),
			Type:        patterns.TypeCredential,
			Severity:    "BLOCK",
			Description: "OpenAI API key",
		},
	}
	det.LoadPatterns(testPatterns)

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
				Name:        patterns.PatternEmail,
				Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
				Type:        patterns.TypePII,
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
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
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

func TestSanitizer_Sanitize_DOBRedaction(t *testing.T) {
	det := detector.New()
	dobPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternDateOfBirth,
		Regex:       regexp.MustCompile(`\b\d{4}[-/.]\d{1,2}[-/.]\d{1,2}\b|\b\d{1,2}[-/.]\d{1,2}[-/.]\d{4}\b`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Date pattern",
	}
	det.LoadPattern(dobPattern)

	s := New(det)

	input := "DOB: 1988-04-12"
	result, err := s.Sanitize(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.SanitizedPrompt, "1988-04-12") {
		t.Error("DOB should have been redacted")
	}
	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_DOB]") {
		t.Errorf("expected [REDACTED_DOB], got: %s", result.SanitizedPrompt)
	}
}

func TestSanitizer_Sanitize_PostalCodeRedaction(t *testing.T) {
	det := detector.New()
	postalPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternPostalCode,
		Regex:       regexp.MustCompile(`(?i)(?:postal[\s._-]*code|zip[\s._-]*code|zip|postcode)\s*[:=]?\s*[A-Za-z0-9]{3,10}(?:[\s-][A-Za-z0-9]{2,4})?`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Postal or ZIP code with contextual keyword",
	}
	det.LoadPattern(postalPattern)

	s := New(det)

	input := "Postal Code: 100271"
	result, err := s.Sanitize(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.SanitizedPrompt, "100271") {
		t.Error("postal code should have been redacted")
	}
	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_POSTAL_CODE]") {
		t.Errorf("expected [REDACTED_POSTAL_CODE], got: %s", result.SanitizedPrompt)
	}
}

func TestSanitizer_Sanitize_EmployeeIDRedaction(t *testing.T) {
	det := detector.New()
	empPattern := &patterns.CompiledPattern{
		Name:        "employee_id",
		Regex:       regexp.MustCompile(`(?i)\b(?:emp|employee|staff|personnel)[-_]?\d{3,10}\b`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Employee ID",
	}
	det.LoadPattern(empPattern)

	s := New(det)

	input := "Employee ID: EMP-44219"
	result, err := s.Sanitize(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.SanitizedPrompt, "EMP-44219") {
		t.Error("employee ID should have been redacted")
	}
	// employee_id is a custom pattern with no specific label, falls back to [REDACTED_PII]
	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_PII]") {
		t.Errorf("expected [REDACTED_PII], got: %s", result.SanitizedPrompt)
	}
}

func TestSanitizer_Sanitize_CombinedPIIInput(t *testing.T) {
	det := detector.New()

	testPatterns := []*patterns.CompiledPattern{
		{
			Name:        patterns.PatternDateOfBirth,
			Regex:       regexp.MustCompile(`\b\d{4}[-/.]\d{1,2}[-/.]\d{1,2}\b|\b\d{1,2}[-/.]\d{1,2}[-/.]\d{4}\b`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Date pattern",
		},
		{
			Name:        patterns.PatternPostalCode,
			Regex:       regexp.MustCompile(`(?i)(?:postal[\s._-]*code|zip[\s._-]*code|zip|postcode)\s*[:=]?\s*[A-Za-z0-9]{3,10}(?:[\s-][A-Za-z0-9]{2,4})?`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Postal or ZIP code",
		},
		{
			Name:        "employee_id",
			Regex:       regexp.MustCompile(`(?i)\b(?:emp|employee|staff|personnel)[-_]?\d{3,10}\b`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Employee ID",
		},
	}
	det.LoadPatterns(testPatterns)

	s := New(det)

	input := "DOB: 1988-04-12, Postal Code: 100271, Employee ID: EMP-44219"
	result, err := s.Sanitize(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Safe {
		t.Error("expected Safe to be false")
	}

	if strings.Contains(result.SanitizedPrompt, "1988-04-12") {
		t.Error("DOB should have been redacted")
	}
	if strings.Contains(result.SanitizedPrompt, "100271") {
		t.Error("postal code should have been redacted")
	}
	if strings.Contains(result.SanitizedPrompt, "EMP-44219") {
		t.Error("employee ID should have been redacted")
	}

	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_DOB]") {
		t.Errorf("expected [REDACTED_DOB] in output, got: %s", result.SanitizedPrompt)
	}
	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_POSTAL_CODE]") {
		t.Errorf("expected [REDACTED_POSTAL_CODE] in output, got: %s", result.SanitizedPrompt)
	}
	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_PII]") {
		t.Errorf("expected [REDACTED_PII] in output, got: %s", result.SanitizedPrompt)
	}

	t.Logf("Sanitized: %s", result.SanitizedPrompt)
}

func TestSanitizer_GetRedactionLabel(t *testing.T) {
	s := New(detector.New())

	tests := []struct {
		patternName string
		patternType string
		expected    string
	}{
		{patterns.PatternEmail, patterns.TypePII, "[REDACTED_EMAIL]"},
		{patterns.PatternOpenAIAPIKey, patterns.TypeCredential, "[REDACTED_API_KEY]"},
		{patterns.PatternNigerianBVN, patterns.TypePII, "[REDACTED_BVN]"},
		{patterns.PatternCreditCard, patterns.TypeFinancial, "[REDACTED_CARD]"},
		{patterns.PatternDateOfBirth, patterns.TypePII, "[REDACTED_DOB]"},
		{patterns.PatternPostalCode, patterns.TypePII, "[REDACTED_POSTAL_CODE]"},
		{"unknown_pattern", patterns.TypePII, "[REDACTED_PII]"},
		{"unknown_pattern", patterns.TypeCredential, "[REDACTED_CREDENTIAL]"},
		{"unknown_pattern", patterns.TypeFinancial, "[REDACTED_FINANCIAL]"},
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

	testPatterns := []*patterns.CompiledPattern{
		{
			Name:        patterns.PatternEmail,
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        patterns.TypePII,
			Severity:    "WARN",
			Description: "Email address",
		},
		{
			Name:        patterns.PatternPostgresConn,
			Regex:       regexp.MustCompile(`postgres://[^\s]+`),
			Type:        patterns.TypeCredential,
			Severity:    "BLOCK",
			Description: "PostgreSQL connection string",
		},
		{
			Name:        patterns.PatternNigerianBVN,
			Regex:       regexp.MustCompile(`\b\d{11}\b`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Nigerian BVN",
		},
	}
	det.LoadPatterns(testPatterns)

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

func TestSanitizer_SanitizeWithSession_TokenizeMode(t *testing.T) {
	det := detector.New()
	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Email address",
	}
	det.LoadPattern(emailPattern)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	s := NewWithTokenizer(det, tok, vm, "tokenize")

	input := "Contact john@example.com"
	sessionID := "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
	result, err := s.SanitizeWithSession(input, sessionID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SessionID != sessionID {
		t.Errorf("expected session ID %s, got %s", sessionID, result.SessionID)
	}

	if result.TokenizedCount != 1 {
		t.Errorf("expected tokenized count to be 1, got %d", result.TokenizedCount)
	}

	if strings.Contains(result.SanitizedPrompt, "john@example.com") {
		t.Error("email should have been tokenized")
	}
}

func TestSanitizer_SanitizeWithSession_RedactMode(t *testing.T) {
	det := detector.New()
	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Email address",
	}
	det.LoadPattern(emailPattern)

	s := NewWithTokenizer(det, nil, nil, "redact")

	input := "Contact john@example.com"
	sessionID := "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
	result, err := s.SanitizeWithSession(input, sessionID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TokenizedCount != 0 {
		t.Errorf("expected tokenized count to be 0, got %d", result.TokenizedCount)
	}

	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_EMAIL]") {
		t.Errorf("expected redaction marker, got: %s", result.SanitizedPrompt)
	}
}

func TestSanitizer_SanitizeWithSession_CredentialAlwaysRedacted(t *testing.T) {
	det := detector.New()
	testPatterns := []*patterns.CompiledPattern{
		{
			Name:        patterns.PatternEmail,
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Email address",
		},
		{
			Name:        patterns.PatternOpenAIAPIKey,
			Regex:       regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),
			Type:        patterns.TypeCredential,
			Severity:    "BLOCK",
			Description: "OpenAI API key",
		},
	}
	det.LoadPatterns(testPatterns)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	s := NewWithTokenizer(det, tok, vm, "tokenize")

	input := "Email john@example.com and key sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd"
	sessionID := "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
	result, err := s.SanitizeWithSession(input, sessionID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_API_KEY]") {
		t.Errorf("expected credential to be redacted, got: %s", result.SanitizedPrompt)
	}

	if strings.Contains(result.SanitizedPrompt, "sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd") {
		t.Error("credential should not appear in sanitized output")
	}

	if result.TokenizedCount != 1 {
		t.Errorf("expected tokenized count to be 1, got %d", result.TokenizedCount)
	}
}

func TestSanitizer_SanitizeWithSession_DeterministicReuse(t *testing.T) {
	det := detector.New()
	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Email address",
	}
	det.LoadPattern(emailPattern)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	s := NewWithTokenizer(det, tok, vm, "tokenize")

	input := "Contact john@example.com"
	sessionID := "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"

	first, err := s.SanitizeWithSession(input, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	second, err := s.SanitizeWithSession(input, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first.SanitizedPrompt != second.SanitizedPrompt {
		t.Errorf("expected deterministic token reuse, got %s and %s", first.SanitizedPrompt, second.SanitizedPrompt)
	}
}

func TestSanitizer_Sanitize_BackwardCompatibility(t *testing.T) {
	det := detector.New()
	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Email address",
	}
	det.LoadPattern(emailPattern)

	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(1 * time.Minute)
	s := NewWithTokenizer(det, tok, vm, "tokenize")

	input := "Contact john@example.com"
	result, err := s.Sanitize(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.SanitizedPrompt, "[REDACTED_EMAIL]") {
		t.Errorf("expected redaction marker, got: %s", result.SanitizedPrompt)
	}
}
