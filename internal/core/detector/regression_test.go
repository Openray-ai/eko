package detector

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"eko/internal/core/patterns"
)

func newDefaultDetector(t *testing.T) *Detector {
	t.Helper()
	d := New()
	loader := patterns.NewLoader(filepath.Join("..", "..", "..", "patterns", "default"), filepath.Join("..", "..", "..", "patterns", "custom"))
	compiled, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("failed to load patterns: %v", err)
	}
	d.LoadPatterns(compiled)
	return d
}

func hasPattern(violations []Violation, patternName string) bool {
	for _, v := range violations {
		if v.Pattern == patternName {
			return true
		}
	}
	return false
}

func firstPattern(violations []Violation, patternName string) (Violation, bool) {
	for _, v := range violations {
		if v.Pattern == patternName {
			return v, true
		}
	}
	return Violation{}, false
}

func TestDetector_Regression_Problem01_OpenAIProjKeyNoSwiftFalsePositive(t *testing.T) {
	d := newDefaultDetector(t)
	input := "key=sk-proj-1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	violations, err := d.Detect(input)
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternOpenAIAPIKey) {
		t.Fatalf("expected %s detection, got %+v", patterns.PatternOpenAIAPIKey, violations)
	}
	if hasPattern(violations, patterns.PatternSwiftCode) {
		t.Fatalf("did not expect swift false positive, got %+v", violations)
	}
}

func TestDetector_Regression_Problem03_AWSTempKeyNoSwiftFalsePositive(t *testing.T) {
	d := newDefaultDetector(t)
	input := "aws_temp=ASIAQWERTYUIOPLKJHG"
	violations, err := d.Detect(input)
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternAWSAccessKey) {
		t.Fatalf("expected %s detection, got %+v", patterns.PatternAWSAccessKey, violations)
	}
	if hasPattern(violations, patterns.PatternSwiftCode) {
		t.Fatalf("did not expect swift false positive, got %+v", violations)
	}
}

func TestDetector_Regression_Problem04_CardsWithSeparatorsDetected(t *testing.T) {
	d := newDefaultDetector(t)
	inputs := []string{
		"card=4111 1111 1111 1111",
		"card=4111-1111-1111-1111",
	}
	for _, input := range inputs {
		violations, err := d.Detect(input)
		if err != nil {
			t.Fatalf("detect failed: %v", err)
		}
		if !hasPattern(violations, patterns.PatternCreditCard) {
			t.Fatalf("expected card detection for %q, got %+v", input, violations)
		}
	}
}

func TestDetector_Regression_Problem05_UrlEncodedEmailDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("email=john.doe%40example.com")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternEmail) {
		t.Fatalf("expected email detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem06_ConfusableEmailDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("email=jоhn.doe@exаmple.com")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternEmail) {
		t.Fatalf("expected email detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem07_SpacedPhoneDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("phone=+234 801 234 5678")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternNigerianPhone) {
		t.Fatalf("expected nigerian phone detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem08_LowerSpacedIBANDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("iban=gb29 nwbk 6016 1331 9268 19")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternIBAN) {
		t.Fatalf("expected iban detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem09_ObfuscatedPasswordKeyDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("p@ssword=Sup3rSecret!")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternPasswordVar) {
		t.Fatalf("expected password detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem10_QuotedPasswordWithSpacesDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect(`password="correct horse battery staple"`)
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternPasswordVar) {
		t.Fatalf("expected password detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem11_SplitEmailDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("email parts: john.doe + @example.com")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternEmail) {
		t.Fatalf("expected split email detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem12_Base64EncodedSecretDetected(t *testing.T) {
	d := newDefaultDetector(t)
	encoded := base64.StdEncoding.EncodeToString([]byte("postgres://admin:Secret123@db.internal:5432/payments"))
	violations, err := d.Detect("blob=" + encoded)
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, "base64_encoded_secret") {
		t.Fatalf("expected base64_encoded_secret detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem13_EscapedCredentialURIDetected(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("postgres://admin:Very\nSecret@db.internal:5432/app")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	v, ok := firstPattern(violations, patterns.PatternPostgresConn)
	if !ok {
		t.Fatalf("expected postgres detection, got %+v", violations)
	}
	if !strings.Contains(v.Matched, "@db.internal") {
		t.Fatalf("expected full-ish credential match to include host, got %q", v.Matched)
	}
}

func TestDetector_Regression_Problem14_BaselineEmailMustHold(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("email=jane.doe@example.com")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternEmail) {
		t.Fatalf("expected baseline email detection, got %+v", violations)
	}
}

func TestDetector_Regression_Problem15_BaselinePostgresMustHold(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("postgres://admin:Secret123@db.internal:5432/payments")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternPostgresConn) {
		t.Fatalf("expected baseline postgres detection, got %+v", violations)
	}
}

func TestDetector_DriftGuard_RandomBase64NotFlagged(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("blob=VGhpcyBpcyBub3QgYSBzZWNyZXQgY3JlZGVudGlhbA==")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if hasPattern(violations, "base64_encoded_secret") {
		t.Fatalf("unexpected base64 false positive: %+v", violations)
	}
}

func TestDetector_DriftGuard_InvalidLuhnNotFlaggedAsCard(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("card=4111 1111 1111 1112")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if hasPattern(violations, patterns.PatternCreditCard) {
		t.Fatalf("unexpected credit card false positive: %+v", violations)
	}
}

func TestDetector_DriftGuard_SwiftStillDetectedForValidBIC(t *testing.T) {
	d := newDefaultDetector(t)
	violations, err := d.Detect("swift=DEUTDEFF500")
	if err != nil {
		t.Fatalf("detect failed: %v", err)
	}
	if !hasPattern(violations, patterns.PatternSwiftCode) {
		t.Fatalf("expected swift detection for valid BIC, got %+v", violations)
	}
}
