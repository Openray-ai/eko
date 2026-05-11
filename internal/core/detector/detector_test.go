package detector

import (
	"eko/internal/core/patterns"
	"math/bits"
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

	pattern := &patterns.CompiledPattern{
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
	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
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
	if v.Pattern != patterns.PatternEmail {
		t.Errorf("expected pattern 'email', got '%s'", v.Pattern)
	}
	if v.Matched != "john@example.com" {
		t.Errorf("expected matched 'john@example.com', got '%s'", v.Matched)
	}
	if v.Type != patterns.TypePII {
		t.Errorf("expected type 'pii', got '%s'", v.Type)
	}
	if v.Severity != "WARN" {
		t.Errorf("expected severity 'WARN', got '%s'", v.Severity)
	}
}

func TestDetector_Detect_MultipleMatches(t *testing.T) {
	d := New()

	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
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
	testPatterns := []*patterns.CompiledPattern{
		{
			Name:        patterns.PatternEmail,
			Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
			Type:        patterns.TypePII,
			Severity:    "WARN",
			Description: "Email address",
		},
		{
			Name:        patterns.PatternOpenAIAPIKey,
			Regex:       regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),
			Type:        patterns.TypeCredential,
			Severity:    "BLOCK",
			Description: "OpenAI API key",
		},
		{
			Name:        "phone",
			Regex:       regexp.MustCompile(`\+\d{1,3}\s?\d{9,10}`),
			Type:        patterns.TypePII,
			Severity:    "WARN",
			Description: "Phone number",
		},
	}

	d.LoadPatterns(testPatterns)

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
	requiredPatterns := []string{patterns.PatternEmail, patterns.PatternOpenAIAPIKey}
	for _, required := range requiredPatterns {
		if !foundPatterns[required] {
			t.Errorf("pattern '%s' was not detected", required)
		}
	}
}

func TestDetector_Detect_Deduplication(t *testing.T) {
	d := New()

	// Load two overlapping patterns
	dedupPatterns := []*patterns.CompiledPattern{
		{
			Name:        "generic_number",
			Regex:       regexp.MustCompile(`\d{11}`),
			Type:        patterns.TypePII,
			Severity:    "LOG",
			Description: "11 digit number",
		},
		{
			Name:        patterns.PatternNigerianBVN,
			Regex:       regexp.MustCompile(`\b\d{11}\b`),
			Type:        patterns.TypePII,
			Severity:    "BLOCK",
			Description: "Nigerian BVN",
		},
	}

	d.LoadPatterns(dedupPatterns)

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

func TestDetector_Deduplicate_DoesNotLetBridgeSpanSuppressNonOverlaps(t *testing.T) {
	d := New()
	violations := []Violation{
		{Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "a", Matched: "aaaaaaaaaa", Position: 0, End: 10},
		{Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "bridge", Matched: "bbbbbbbbbbb", Position: 9, End: 20},
		{Type: patterns.TypePII, Severity: patterns.SeverityBlock, Pattern: "c", Matched: "ccccccccccc", Position: 19, End: 30},
	}

	deduped := d.deduplicateViolations(violations)
	if len(deduped) != 2 {
		t.Fatalf("expected A and C to survive bridge dedupe, got %+v", deduped)
	}
	found := map[string]bool{}
	for _, v := range deduped {
		found[v.Pattern] = true
	}
	if !found["a"] || !found["c"] || found["bridge"] {
		t.Fatalf("expected only non-overlapping higher-priority endpoints, got %+v", deduped)
	}
}

func TestDetector_Deduplicate_KeepsLargeNonOverlappingSet(t *testing.T) {
	d := New()
	const count = 5000
	violations := make([]Violation, 0, count)
	for i := 0; i < count; i++ {
		severity := patterns.SeverityWarn
		if i%2 == 0 {
			severity = patterns.SeverityBlock
		}
		violations = append(violations, Violation{
			Type:     patterns.TypePII,
			Severity: severity,
			Pattern:  "non_overlapping",
			Matched:  "value",
			Position: i * 10,
			End:      i*10 + 5,
		})
	}

	deduped := d.deduplicateViolations(violations)
	if len(deduped) != count {
		t.Fatalf("expected all non-overlapping violations to survive, got %d", len(deduped))
	}
	for i := 1; i < len(deduped); i++ {
		if deduped[i-1].Position >= deduped[i].Position {
			t.Fatalf("expected deduped violations sorted by position at index %d: %+v then %+v", i, deduped[i-1], deduped[i])
		}
	}
}

func TestViolationSpanIndex_BalancesAscendingInserts(t *testing.T) {
	const count = 1024
	index := violationSpanIndex{}
	for i := 0; i < count; i++ {
		index.insert(Violation{Position: i * 10, End: i*10 + 5})
	}

	maxReasonableAVLHeight := bits.Len(uint(count)) * 2
	if index.root.height > maxReasonableAVLHeight {
		t.Fatalf("expected balanced span index height <= %d, got %d", maxReasonableAVLHeight, index.root.height)
	}
}

func TestDetector_Detect_EmptyInput(t *testing.T) {
	d := New()

	emailPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternEmail,
		Regex:       regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		Type:        patterns.TypePII,
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

	patternList := []*patterns.CompiledPattern{
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

	d.LoadPatterns(patternList)

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

func TestDetector_Detect_DateOfBirth(t *testing.T) {
	d := New()

	dobPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternDateOfBirth,
		Regex:       regexp.MustCompile(`\b\d{4}[-/.]\d{1,2}[-/.]\d{1,2}\b|\b\d{1,2}[-/.]\d{1,2}[-/.]\d{4}\b`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Date pattern (date of birth or similar)",
	}
	d.LoadPattern(dobPattern)

	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		{"YYYY-MM-DD", "DOB: 1988-04-12", true},
		{"DD/MM/YYYY", "Born on 12/04/1988", true},
		{"MM.DD.YYYY", "Date: 04.12.1988", true},
		{"YYYY/M/D", "Date: 1988/4/12", true},
		{"no match plain text", "Hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := d.Detect(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.matches && len(violations) == 0 {
				t.Errorf("expected DOB match in %q, got none", tt.input)
			}
			if !tt.matches && len(violations) > 0 {
				t.Errorf("expected no DOB match in %q, got %d", tt.input, len(violations))
			}
		})
	}
}

func TestDetector_Detect_PostalCode(t *testing.T) {
	d := New()

	postalPattern := &patterns.CompiledPattern{
		Name:        patterns.PatternPostalCode,
		Regex:       regexp.MustCompile(`(?i)(?:postal[\s._-]*code|zip[\s._-]*code|zip|postcode)\s*[:=]?\s*[A-Za-z0-9]{3,10}(?:[\s-][A-Za-z0-9]{2,4})?`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Postal or ZIP code with contextual keyword",
	}
	d.LoadPattern(postalPattern)

	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		{"postal code numeric", "Postal Code: 100271", true},
		{"zip code", "zip code: 90210", true},
		{"ZIP short", "ZIP: 10001-4321", true},
		{"postcode UK", "Postcode: SW1A 1AA", true},
		{"bare number no keyword", "100271", false},
		{"no match plain text", "Hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := d.Detect(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.matches && len(violations) == 0 {
				t.Errorf("expected postal code match in %q, got none", tt.input)
			}
			if !tt.matches && len(violations) > 0 {
				t.Errorf("expected no postal code match in %q, got %d", tt.input, len(violations))
			}
		})
	}
}

func TestDetector_Detect_EmployeeID(t *testing.T) {
	d := New()

	empPattern := &patterns.CompiledPattern{
		Name:        "employee_id",
		Regex:       regexp.MustCompile(`(?i)\b(?:emp|employee|staff|personnel)[-_]?\d{3,10}\b`),
		Type:        patterns.TypePII,
		Severity:    "BLOCK",
		Description: "Employee ID",
	}
	d.LoadPattern(empPattern)

	tests := []struct {
		name    string
		input   string
		matches bool
	}{
		{"EMP-nnnnn", "Employee ID: EMP-44219", true},
		{"emp_nnnnn", "ID is emp_12345", true},
		{"employee followed by digits", "employee12345", true},
		{"staff ID", "staff-001", true},
		{"personnel ID", "personnel999999", true},
		{"no match plain text", "Hello world", false},
		{"too few digits", "EMP-12", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := d.Detect(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.matches && len(violations) == 0 {
				t.Errorf("expected employee ID match in %q, got none", tt.input)
			}
			if !tt.matches && len(violations) > 0 {
				t.Errorf("expected no employee ID match in %q, got %d", tt.input, len(violations))
			}
		})
	}
}

func TestDetector_Detect_Positions(t *testing.T) {
	d := New()

	pattern := &patterns.CompiledPattern{
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
