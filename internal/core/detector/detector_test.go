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

func TestDetector_Detect_NoPatterns(t *testing.T) {
	d := New()

	violations, err := d.Detect("test input")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should return empty slice when no patterns loaded
	if len(violations) != 0 {
		t.Errorf("expected 0 violations with no patterns loaded, got %d", len(violations))
	}
}

func TestDetector_Detect_SinglePattern(t *testing.T) {
	d := New()

	// Load an email pattern
	emailPattern := &CompiledPattern{
		Name:        "email",
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        "pii",
		Severity:    "WARN",
		Description: "Email address",
	}
	d.LoadPattern(emailPattern)

	input := "My email is john@example.com"
	violations, err := d.Detect(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}

	v := violations[0]
	if v.Pattern != "email" {
		t.Errorf("expected pattern 'email', got '%s'", v.Pattern)
	}
	if v.Matched != "john@example.com" {
		t.Errorf("expected matched 'john@example.com', got '%s'", v.Matched)
	}
	if v.Type != "pii" {
		t.Errorf("expected type 'pii', got '%s'", v.Type)
	}
	if v.Severity != "WARN" {
		t.Errorf("expected severity 'WARN', got '%s'", v.Severity)
	}
}

func TestDetector_Detect_MultipleMatches(t *testing.T) {
	d := New()

	emailPattern := &CompiledPattern{
		Name:        "email",
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        "pii",
		Severity:    "WARN",
		Description: "Email address",
	}
	d.LoadPattern(emailPattern)

	input := "Contact john@example.com or jane@example.org"
	violations, err := d.Detect(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}

	// Check both emails were detected
	emails := map[string]bool{
		"john@example.com": false,
		"jane@example.org": false,
	}

	for _, v := range violations {
		if _, exists := emails[v.Matched]; exists {
			emails[v.Matched] = true
		}
	}

	for email, found := range emails {
		if !found {
			t.Errorf("email %s was not detected", email)
		}
	}
}

func TestDetector_Detect_MultiplePatterns(t *testing.T) {
	d := New()

	// Load multiple patterns
	patterns := []*CompiledPattern{
		{
			Name:        "email",
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        "pii",
			Severity:    "WARN",
			Description: "Email address",
		},
		{
			Name:        "openai_api_key",
			Regex:       regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),
			Type:        "credential",
			Severity:    "BLOCK",
			Description: "OpenAI API key",
		},
		{
			Name:        "phone",
			Regex:       regexp.MustCompile(`\+\d{1,3}\s?\d{9,10}`),
			Type:        "pii",
			Severity:    "WARN",
			Description: "Phone number",
		},
	}

	d.LoadPatterns(patterns)

	// sk- followed by exactly 48 alphanumeric characters
	input := "Email: john@example.com, API: sk-abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHabcd, Phone: +234 9012345678"
	violations, err := d.Detect(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should detect at least email and API key (phone pattern may or may not match depending on regex)
	if len(violations) < 2 {
		t.Fatalf("expected at least 2 violations, got %d", len(violations))
	}

	// Verify key patterns were detected
	foundPatterns := make(map[string]bool)
	for _, v := range violations {
		foundPatterns[v.Pattern] = true
	}

	// At minimum, email and API key should be detected
	requiredPatterns := []string{"email", "openai_api_key"}
	for _, required := range requiredPatterns {
		if !foundPatterns[required] {
			t.Errorf("pattern '%s' was not detected", required)
		}
	}
}

func TestDetector_Detect_Deduplication(t *testing.T) {
	d := New()

	// Load two overlapping patterns
	patterns := []*CompiledPattern{
		{
			Name:        "generic_number",
			Regex:       regexp.MustCompile(`\d{11}`),
			Type:        "pii",
			Severity:    "LOG",
			Description: "11 digit number",
		},
		{
			Name:        "nigerian_bvn",
			Regex:       regexp.MustCompile(`\b\d{11}\b`),
			Type:        "pii",
			Severity:    "BLOCK",
			Description: "Nigerian BVN",
		},
	}

	d.LoadPatterns(patterns)

	input := "My BVN is 12345678901"
	violations, err := d.Detect(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should deduplicate and keep only one (the higher severity one)
	if len(violations) > 1 {
		// Check if violations actually overlap
		for i := 0; i < len(violations)-1; i++ {
			for j := i + 1; j < len(violations); j++ {
				if d.overlaps(violations[i], violations[j]) {
					t.Errorf("found overlapping violations that should have been deduplicated")
				}
			}
		}
	}
}

func TestDetector_Detect_EmptyInput(t *testing.T) {
	d := New()

	emailPattern := &CompiledPattern{
		Name:        "email",
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        "pii",
		Severity:    "WARN",
		Description: "Email address",
	}
	d.LoadPattern(emailPattern)

	violations, err := d.Detect("")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(violations) != 0 {
		t.Errorf("expected 0 violations for empty input, got %d", len(violations))
	}
}

func TestDetector_LoadPatterns(t *testing.T) {
	d := New()

	patterns := []*CompiledPattern{
		{
			Name:        "pattern1",
			Regex:       regexp.MustCompile(`test1`),
			Type:        "test",
			Severity:    "BLOCK",
			Description: "Test pattern 1",
		},
		{
			Name:        "pattern2",
			Regex:       regexp.MustCompile(`test2`),
			Type:        "test",
			Severity:    "WARN",
			Description: "Test pattern 2",
		},
		{
			Name:        "pattern3",
			Regex:       regexp.MustCompile(`test3`),
			Type:        "test",
			Severity:    "LOG",
			Description: "Test pattern 3",
		},
	}

	d.LoadPatterns(patterns)

	if count := d.GetPatternCount(); count != 3 {
		t.Errorf("expected 3 patterns loaded, got %d", count)
	}

	names := d.GetPatternNames()
	if len(names) != 3 {
		t.Errorf("expected 3 pattern names, got %d", len(names))
	}

	// Verify names are sorted
	expectedNames := []string{"pattern1", "pattern2", "pattern3"}
	for i, name := range names {
		if name != expectedNames[i] {
			t.Errorf("expected name[%d] to be '%s', got '%s'", i, expectedNames[i], name)
		}
	}
}

func TestDetector_GetSeverityPriority(t *testing.T) {
	d := New()

	tests := []struct {
		severity string
		priority int
	}{
		{"BLOCK", 3},
		{"WARN", 2},
		{"LOG", 1},
		{"UNKNOWN", 0},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			priority := d.getSeverityPriority(tt.severity)
			if priority != tt.priority {
				t.Errorf("expected priority %d for severity '%s', got %d", tt.priority, tt.severity, priority)
			}
		})
	}
}

func TestDetector_Overlaps(t *testing.T) {
	d := New()

	tests := []struct {
		name     string
		v1       Violation
		v2       Violation
		overlaps bool
	}{
		{
			name:     "completely separate",
			v1:       Violation{Position: 0, End: 5},
			v2:       Violation{Position: 10, End: 15},
			overlaps: false,
		},
		{
			name:     "v1 contains v2",
			v1:       Violation{Position: 0, End: 20},
			v2:       Violation{Position: 5, End: 10},
			overlaps: true,
		},
		{
			name:     "v2 contains v1",
			v1:       Violation{Position: 5, End: 10},
			v2:       Violation{Position: 0, End: 20},
			overlaps: true,
		},
		{
			name:     "partial overlap",
			v1:       Violation{Position: 0, End: 10},
			v2:       Violation{Position: 5, End: 15},
			overlaps: true,
		},
		{
			name:     "adjacent but not overlapping",
			v1:       Violation{Position: 0, End: 5},
			v2:       Violation{Position: 5, End: 10},
			overlaps: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.overlaps(tt.v1, tt.v2)
			if result != tt.overlaps {
				t.Errorf("expected overlaps=%v, got %v", tt.overlaps, result)
			}
		})
	}
}

func TestDetector_Detect_Positions(t *testing.T) {
	d := New()

	pattern := &CompiledPattern{
		Name:        "test",
		Regex:       regexp.MustCompile(`test`),
		Type:        "test",
		Severity:    "BLOCK",
		Description: "Test pattern",
	}
	d.LoadPattern(pattern)

	input := "This is a test string with test word twice"
	violations, err := d.Detect(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(violations))
	}

	// Verify positions are correct and sorted
	if violations[0].Position >= violations[1].Position {
		t.Error("violations are not sorted by position")
	}

	// Verify actual positions
	for _, v := range violations {
		actualText := input[v.Position:v.End]
		if actualText != v.Matched {
			t.Errorf("position mismatch: extracted '%s', violation has '%s'", actualText, v.Matched)
		}
	}
}
