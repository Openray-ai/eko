package core

import (
	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/sanitizer"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEndToEnd_LoadDetectSanitize tests the complete flow
func TestEndToEnd_LoadDetectSanitize(t *testing.T) {
	// Create temporary pattern file
	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "BLOCK"
    description: "Email address"
  - name: "openai_api_key"
    regex: "sk-[a-zA-Z0-9]{48}"
    type: "credential"
    severity: "BLOCK"
    description: "OpenAI API key"
`

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to create pattern file: %v", err)
	}

	// 1. Load patterns
	loader := patterns.NewLoader(yamlPath, "")
	compiledPatterns, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}

	if len(compiledPatterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(compiledPatterns))
	}

	// 2. Create detector and load patterns
	det := detector.New()
	det.LoadPatterns(compiledPatterns)

	if det.GetPatternCount() != 2 {
		t.Fatalf("expected detector to have 2 patterns, got %d", det.GetPatternCount())
	}

	// 3. Create sanitizer
	san := sanitizer.New(det)

	// 4. Test sanitization
	// sk- followed by exactly 48 alphanumeric characters
	input := "My email is john@example.com and my API key is sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd"
	result, err := san.Sanitize(input)

	if err != nil {
		t.Fatalf("sanitization failed: %v", err)
	}

	// Verify results
	if result.Safe {
		t.Error("result should not be safe")
	}

	if len(result.Violations) != 2 {
		t.Errorf("expected 2 violations, got %d", len(result.Violations))
	}

	if strings.Contains(result.SanitizedPrompt, "john@example.com") {
		t.Error("email should be redacted")
	}

	if strings.Contains(result.SanitizedPrompt, "sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd") {
		t.Error("API key should be redacted")
	}

	t.Logf("Original: %s", input)
	t.Logf("Sanitized: %s", result.SanitizedPrompt)
	t.Logf("Processing time: %.2fms", result.ProcessingTimeMs)
}

// TestEndToEnd_RealWorldScenario tests with realistic data from README
func TestEndToEnd_RealWorldScenario(t *testing.T) {
	// Create comprehensive pattern file
	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "postgres_connection"
    regex: "postgres://[^\\s]+"
    type: "credential"
    severity: "BLOCK"
    description: "PostgreSQL connection string"
  - name: "nigerian_bvn"
    regex: "\\b\\d{11}\\b"
    type: "pii"
    severity: "BLOCK"
    description: "Nigerian BVN"
  - name: "nigerian_account"
    regex: "\\b\\d{10}\\b"
    type: "financial"
    severity: "BLOCK"
    description: "Nigerian bank account"
  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "WARN"
    description: "Email address"
`

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to create pattern file: %v", err)
	}

	// Setup
	loader := patterns.NewLoader(yamlPath, "")
	compiledPatterns, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}

	det := detector.New()
	det.LoadPatterns(compiledPatterns)
	san := sanitizer.New(det)

	// Test scenarios from README
	testCases := []struct {
		name     string
		input    string
		mustFind []string // Patterns that must be detected
		mustHide []string // Strings that must be redacted
	}{
		{
			name:     "Database connection leak",
			input:    "Debug this error: postgres://admin:SecureP@ss123@prod-db.company.com",
			mustFind: []string{"postgres_connection"},
			mustHide: []string{"SecureP@ss123", "prod-db.company.com"},
		},
		{
			name:     "BVN and account leak",
			input:    "Analyze customer feedback from BVN 12345678901 and account 0123456789",
			mustFind: []string{"nigerian_bvn", "nigerian_account"},
			mustHide: []string{"12345678901", "0123456789"},
		},
		{
			name:     "Email warning",
			input:    "Contact support at support@example.com for help",
			mustFind: []string{"email"},
			mustHide: []string{"support@example.com"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := san.Sanitize(tc.input)
			if err != nil {
				t.Fatalf("sanitization failed: %v", err)
			}

			// Verify patterns were detected
			foundPatterns := make(map[string]bool)
			for _, v := range result.Violations {
				foundPatterns[v.Pattern] = true
			}

			for _, pattern := range tc.mustFind {
				if !foundPatterns[pattern] {
					t.Errorf("expected to find pattern '%s'", pattern)
				}
			}

			// Verify sensitive data was hidden
			for _, sensitive := range tc.mustHide {
				if strings.Contains(result.SanitizedPrompt, sensitive) {
					t.Errorf("sensitive data '%s' should be redacted", sensitive)
				}
			}

			t.Logf("  Original: %s", tc.input)
			t.Logf("  Sanitized: %s", result.SanitizedPrompt)
			t.Logf("  Violations: %d", len(result.Violations))
		})
	}
}

// TestEndToEnd_Performance tests performance with many patterns
func TestEndToEnd_Performance(t *testing.T) {
	// Create pattern file with many patterns
	tmpDir := t.TempDir()
	var yamlBuilder strings.Builder
	yamlBuilder.WriteString("patterns:\n")

	// Add 50 patterns
	for i := 0; i < 50; i++ {
		yamlBuilder.WriteString(fmt.Sprintf("  - name: \"pattern_%d\"\n", i))
		yamlBuilder.WriteString(fmt.Sprintf("    regex: \"pattern%d\"\n", i))
		yamlBuilder.WriteString("    type: \"pii\"\n")
		yamlBuilder.WriteString("    severity: \"BLOCK\"\n")
		yamlBuilder.WriteString("    description: \"Test\"\n")
	}

	// Add real patterns
	yamlBuilder.WriteString(`  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "WARN"
    description: "Email"
`)

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlBuilder.String()), 0644)
	if err != nil {
		t.Fatalf("failed to create pattern file: %v", err)
	}

	// Setup
	loader := patterns.NewLoader(yamlPath, "")
	compiledPatterns, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}

	det := detector.New()
	det.LoadPatterns(compiledPatterns)
	san := sanitizer.New(det)

	// Test with moderate-sized input
	input := "This is a test email: john@example.com with some other text that doesn't match anything"

	result, err := san.Sanitize(input)
	if err != nil {
		t.Fatalf("sanitization failed: %v", err)
	}

	// Performance check: should be fast even with 50+ patterns
	if result.ProcessingTimeMs > 50 {
		t.Logf("Warning: Processing took %.2fms with %d patterns (target <50ms)",
			result.ProcessingTimeMs, det.GetPatternCount())
	}

	t.Logf("Processed with %d patterns in %.2fms", det.GetPatternCount(), result.ProcessingTimeMs)
}

// TestEndToEnd_MultipleFiles tests loading from multiple pattern files
func TestEndToEnd_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create default patterns
	defaultYAML := `patterns:
  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "WARN"
    description: "Email"
`

	// Create custom patterns
	customDir := filepath.Join(tmpDir, "custom")
	err := os.Mkdir(customDir, 0755)
	if err != nil {
		t.Fatalf("failed to create custom dir: %v", err)
	}

	customYAML := `patterns:
  - name: "employee_id"
    regex: "EMP-[0-9]{6}"
    type: "pii"
    severity: "BLOCK"
    description: "Employee ID"
`

	err = os.WriteFile(filepath.Join(tmpDir, "default.yaml"), []byte(defaultYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create default yaml: %v", err)
	}

	err = os.WriteFile(filepath.Join(customDir, "custom.yaml"), []byte(customYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create custom yaml: %v", err)
	}

	// Setup with both default and custom
	loader := patterns.NewLoader(filepath.Join(tmpDir, "default.yaml"), customDir)
	compiledPatterns, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}

	if len(compiledPatterns) != 2 {
		t.Fatalf("expected 2 patterns (1 default + 1 custom), got %d", len(compiledPatterns))
	}

	det := detector.New()
	det.LoadPatterns(compiledPatterns)
	san := sanitizer.New(det)

	// Test with both pattern types
	input := "Contact john@example.com or employee EMP-123456"
	result, err := san.Sanitize(input)

	if err != nil {
		t.Fatalf("sanitization failed: %v", err)
	}

	if len(result.Violations) != 2 {
		t.Errorf("expected 2 violations (email + employee ID), got %d", len(result.Violations))
	}

	if strings.Contains(result.SanitizedPrompt, "EMP-123456") {
		t.Error("employee ID should be redacted")
	}

	t.Logf("Loaded patterns from default and custom directories")
	t.Logf("Original: %s", input)
	t.Logf("Sanitized: %s", result.SanitizedPrompt)
}

// TestEndToEnd_DiffGeneration tests the diff formatter integration
func TestEndToEnd_DiffGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "BLOCK"
    description: "Email"
`

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to create pattern file: %v", err)
	}

	// Setup
	loader := patterns.NewLoader(yamlPath, "")
	compiledPatterns, _ := loader.LoadAll()
	det := detector.New()
	det.LoadPatterns(compiledPatterns)
	san := sanitizer.New(det)

	// Sanitize
	input := "My email is john@example.com"
	result, err := san.Sanitize(input)
	if err != nil {
		t.Fatalf("sanitization failed: %v", err)
	}

	// Generate diff
	diff := sanitizer.GenerateDiff(result)

	if diff == nil {
		t.Fatal("GenerateDiff returned nil")
	}

	if diff.TotalChanges != 1 {
		t.Errorf("expected 1 change, got %d", diff.TotalChanges)
	}

	if diff.OriginalPrompt != input {
		t.Error("diff should contain original prompt")
	}

	if diff.SanitizedPrompt != result.SanitizedPrompt {
		t.Error("diff should contain sanitized prompt")
	}

	// Test formatted outputs
	summary := sanitizer.FormatSummary(result)
	if summary == "" {
		t.Error("summary should not be empty")
	}

	violationsList := sanitizer.FormatViolationsList(result)
	if violationsList == "" {
		t.Error("violations list should not be empty")
	}

	t.Logf("Diff Summary:\n%s", summary)
	t.Logf("\nViolations:\n%s", violationsList)
}

// TestEndToEnd_NoViolations tests that safe input passes through unchanged
func TestEndToEnd_NoViolations(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "BLOCK"
    description: "Email"
`

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to create pattern file: %v", err)
	}

	loader := patterns.NewLoader(yamlPath, "")
	compiledPatterns, _ := loader.LoadAll()
	det := detector.New()
	det.LoadPatterns(compiledPatterns)
	san := sanitizer.New(det)

	input := "This is a completely safe message with no sensitive data"
	result, err := san.Sanitize(input)

	if err != nil {
		t.Fatalf("sanitization failed: %v", err)
	}

	if !result.Safe {
		t.Error("result should be safe")
	}

	if result.SanitizedPrompt != input {
		t.Error("safe input should pass through unchanged")
	}

	if len(result.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(result.Violations))
	}

	if result.RedactedCount != 0 {
		t.Errorf("expected 0 redactions, got %d", result.RedactedCount)
	}
}
