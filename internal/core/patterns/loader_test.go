package patterns

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_New(t *testing.T) {
	loader := NewLoader("./test", "./custom")

	if loader == nil {
		t.Fatal("NewLoader() returned nil")
	}

	if loader.configDir != "./test" {
		t.Errorf("expected configDir './test', got '%s'", loader.configDir)
	}

	if loader.customPatternsDir != "./custom" {
		t.Errorf("expected customPatternsDir './custom', got '%s'", loader.customPatternsDir)
	}

	if loader.cache == nil {
		t.Error("cache not initialized")
	}
}

func TestLoader_ValidatePattern(t *testing.T) {
	loader := NewLoader("", "")

	tests := []struct {
		name      string
		pattern   Pattern
		shouldErr bool
	}{
		{
			name: "valid pattern",
			pattern: Pattern{
				Name:        "test",
				Regex:       "test",
				Type:        TypePII,
				Severity:    SeverityBlock,
				Description: "Test pattern",
			},
			shouldErr: false,
		},
		{
			name: "missing name",
			pattern: Pattern{
				Regex:       "test",
				Type:        TypePII,
				Severity:    SeverityBlock,
				Description: "Test pattern",
			},
			shouldErr: true,
		},
		{
			name: "missing regex",
			pattern: Pattern{
				Name:        "test",
				Type:        TypePII,
				Severity:    SeverityBlock,
				Description: "Test pattern",
			},
			shouldErr: true,
		},
		{
			name: "invalid type",
			pattern: Pattern{
				Name:        "test",
				Regex:       "test",
				Type:        "invalid_type",
				Severity:    SeverityBlock,
				Description: "Test pattern",
			},
			shouldErr: true,
		},
		{
			name: "invalid severity",
			pattern: Pattern{
				Name:        "test",
				Regex:       "test",
				Type:        TypePII,
				Severity:    "INVALID",
				Description: "Test pattern",
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loader.validatePattern(tt.pattern)
			if tt.shouldErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoader_CompilePattern(t *testing.T) {
	loader := NewLoader("", "")

	pattern := Pattern{
		Name:        "email",
		Regex:       `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
		Type:        TypePII,
		Severity:    SeverityWarn,
		Description: "Email address",
	}

	compiled, err := loader.CompilePattern(pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if compiled.Name != "email" {
		t.Errorf("expected name 'email', got '%s'", compiled.Name)
	}

	if compiled.Type != TypePII {
		t.Errorf("expected type '%s', got '%s'", TypePII, compiled.Type)
	}

	if compiled.Severity != SeverityWarn {
		t.Errorf("expected severity '%s', got '%s'", SeverityWarn, compiled.Severity)
	}

	if compiled.Regex == nil {
		t.Error("regex should be compiled")
	}

	// Test that it actually matches
	if !compiled.Regex.MatchString("test@example.com") {
		t.Error("compiled regex should match email")
	}
}

func TestLoader_CompilePattern_InvalidRegex(t *testing.T) {
	loader := NewLoader("", "")

	pattern := Pattern{
		Name:        "invalid",
		Regex:       `[invalid(regex`,
		Type:        TypePII,
		Severity:    SeverityBlock,
		Description: "Invalid pattern",
	}

	_, err := loader.CompilePattern(pattern)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestLoader_CompilePattern_Caching(t *testing.T) {
	loader := NewLoader("", "")

	pattern := Pattern{
		Name:        "test",
		Regex:       "test",
		Type:        TypePII,
		Severity:    SeverityBlock,
		Description: "Test pattern",
	}

	// First compilation
	compiled1, err := loader.CompilePattern(pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second compilation should use cache
	compiled2, err := loader.CompilePattern(pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be the same pointer (cached)
	if compiled1 != compiled2 {
		t.Error("expected cached pattern to be returned")
	}

	// Verify cache size
	if loader.GetCacheSize() != 1 {
		t.Errorf("expected cache size 1, got %d", loader.GetCacheSize())
	}
}

func TestLoader_ClearCache(t *testing.T) {
	loader := NewLoader("", "")

	pattern := Pattern{
		Name:        "test",
		Regex:       "test",
		Type:        TypePII,
		Severity:    SeverityBlock,
		Description: "Test pattern",
	}

	_, err := loader.CompilePattern(pattern)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if loader.GetCacheSize() != 1 {
		t.Error("cache should have 1 entry")
	}

	loader.ClearCache()

	if loader.GetCacheSize() != 0 {
		t.Error("cache should be empty after clear")
	}
}

func TestLoader_LoadYAMLFile(t *testing.T) {
	// Create temporary YAML file
	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "test_pattern"
    regex: "test"
    type: "pii"
    severity: "BLOCK"
    description: "Test pattern"
  - name: "email"
    regex: "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}"
    type: "pii"
    severity: "WARN"
    description: "Email address"
`

	yamlPath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	loader := NewLoader("", "")
	patterns, err := loader.loadYAMLFile(yamlPath)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(patterns))
	}

	if patterns[0].Name != "test_pattern" {
		t.Errorf("expected first pattern name 'test_pattern', got '%s'", patterns[0].Name)
	}

	if patterns[1].Name != "email" {
		t.Errorf("expected second pattern name 'email', got '%s'", patterns[1].Name)
	}
}

func TestLoader_LoadFromDirectory_File(t *testing.T) {
	// Create temporary YAML file
	tmpDir := t.TempDir()
	yamlContent := `patterns:
  - name: "test"
    regex: "test"
    type: "pii"
    severity: "BLOCK"
    description: "Test"
`

	yamlPath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	loader := NewLoader("", "")
	patterns, filesLoaded, err := loader.loadFromDirectory(yamlPath, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filesLoaded != 1 {
		t.Errorf("expected 1 file loaded, got %d", filesLoaded)
	}

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
}

func TestLoader_LoadFromDirectory_Directory(t *testing.T) {
	// Create temporary directory with multiple YAML files
	tmpDir := t.TempDir()

	yamlContent1 := `patterns:
  - name: "pattern1"
    regex: "test1"
    type: "pii"
    severity: "BLOCK"
    description: "Test 1"
`

	yamlContent2 := `patterns:
  - name: "pattern2"
    regex: "test2"
    type: "credential"
    severity: "WARN"
    description: "Test 2"
  - name: "pattern3"
    regex: "test3"
    type: "financial"
    severity: "LOG"
    description: "Test 3"
`

	err := os.WriteFile(filepath.Join(tmpDir, "file1.yaml"), []byte(yamlContent1), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "file2.yml"), []byte(yamlContent2), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Add a non-YAML file that should be ignored
	err = os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("ignored"), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	loader := NewLoader("", "")
	patterns, filesLoaded, err := loader.loadFromDirectory(tmpDir, "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if filesLoaded != 2 {
		t.Errorf("expected 2 files loaded (YAML only), got %d", filesLoaded)
	}

	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns total, got %d", len(patterns))
	}
}

func TestLoader_LoadFromDirectory_NonExistent(t *testing.T) {
	loader := NewLoader("", "")
	_, _, err := loader.loadFromDirectory("/non/existent/path", "test")

	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestLoader_LoadAll(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	defaultDir := filepath.Join(tmpDir, "default")
	customDir := filepath.Join(tmpDir, "custom")

	err := os.Mkdir(defaultDir, 0755)
	if err != nil {
		t.Fatalf("failed to create default dir: %v", err)
	}

	err = os.Mkdir(customDir, 0755)
	if err != nil {
		t.Fatalf("failed to create custom dir: %v", err)
	}

	// Create pattern files
	defaultYAML := `patterns:
  - name: "default_pattern"
    regex: "default"
    type: "pii"
    severity: "BLOCK"
    description: "Default pattern"
`

	customYAML := `patterns:
  - name: "custom_pattern"
    regex: "custom"
    type: "credential"
    severity: "WARN"
    description: "Custom pattern"
`

	err = os.WriteFile(filepath.Join(defaultDir, "default.yaml"), []byte(defaultYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create default yaml: %v", err)
	}

	err = os.WriteFile(filepath.Join(customDir, "custom.yaml"), []byte(customYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create custom yaml: %v", err)
	}

	loader := NewLoader(defaultDir, customDir)
	compiled, err := loader.LoadAll()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compiled) != 2 {
		t.Fatalf("expected 2 compiled patterns, got %d", len(compiled))
	}

	// Verify both patterns were loaded
	patternNames := make(map[string]bool)
	for _, p := range compiled {
		patternNames[p.Name] = true
	}

	if !patternNames["default_pattern"] {
		t.Error("default_pattern not found")
	}

	if !patternNames["custom_pattern"] {
		t.Error("custom_pattern not found")
	}
}

func TestLoader_LoadAll_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()

	// Create YAML with invalid regex
	invalidYAML := `patterns:
  - name: "invalid"
    regex: "[invalid(regex"
    type: "pii"
    severity: "BLOCK"
    description: "Invalid"
  - name: "valid"
    regex: "valid"
    type: "pii"
    severity: "BLOCK"
    description: "Valid"
`

	yamlPath := filepath.Join(tmpDir, "patterns.yaml")
	err := os.WriteFile(yamlPath, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	loader := NewLoader(yamlPath, "")
	compiled, err := loader.LoadAll()

	// Should not error completely, just skip invalid patterns
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have loaded only the valid pattern
	if len(compiled) != 1 {
		t.Errorf("expected 1 compiled pattern (valid one), got %d", len(compiled))
	}

	if compiled[0].Name != "valid" {
		t.Errorf("expected valid pattern to be loaded, got '%s'", compiled[0].Name)
	}
}
