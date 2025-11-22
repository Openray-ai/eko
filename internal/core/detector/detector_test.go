package detector

import (
	"regexp"
	"testing"
)

func TestDetector_New(t *testing.T) {
	d := New()
	if d == nil {
		t.Fatal("New() returned nil")
	}
	if d.patterns == nil {
		t.Error("patterns map not initialized")
	}
}

func TestDetector_LoadPattern(t *testing.T) {
	d := New()

	pattern := &CompiledPattern{
		Name:        "test_pattern",
		Regex:       regexp.MustCompile(`test`),
		Type:        "test",
		Severity:    "BLOCK",
		Description: "Test pattern",
	}

	d.LoadPattern(pattern)

	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.patterns) != 1 {
		t.Errorf("expected 1 pattern, got %d", len(d.patterns))
	}

	loaded, exists := d.patterns["test_pattern"]
	if !exists {
		t.Error("pattern not found in detector")
	}
	if loaded.Name != "test_pattern" {
		t.Errorf("expected name 'test_pattern', got '%s'", loaded.Name)
	}
}

func TestDetector_Detect(t *testing.T) {
	d := New()

	// TODO: Add test cases for detection
	// TODO: Add test cases for concurrent detection
	// TODO: Add test cases for multiple pattern matching

	violations, err := d.Detect("test input")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should return empty slice, not nil
	if len(violations) != 0 {
		t.Errorf("expected 0 violations with no patterns loaded, got %d", len(violations))
	}
}
