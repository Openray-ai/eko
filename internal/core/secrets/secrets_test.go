package secrets

import (
	"testing"

	"eko/internal/core/patterns"
)

const (
	awsSecretValue  = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	awsAccessKeyID  = "AKIAIOSFODNN7EXAMPLE"
	awsSessionToken = "IQoJb3JpZ2luX2VjEDMaCXVzLWVhc3QtMSJHMEUCIQD1ExampleSessionTokenValueWithEnoughEntropyAndLength1234567890AbCdEfGhIjKlMnOpQrStUvWxYz"
)

func TestDetectAWSSecretAccessKeyAssignments(t *testing.T) {
	tests := []string{
		"AWS_SECRET_ACCESS_KEY=" + awsSecretValue,
		"export AWS_SECRET_ACCESS_KEY=" + awsSecretValue,
		"aws_secret_access_key: " + awsSecretValue,
		`"aws_secret_access_key": "` + awsSecretValue + `"`,
		"aws secret access key is " + awsSecretValue,
		"awsSecretAccessKey = '" + awsSecretValue + "'",
		"SecretAccessKey=" + awsSecretValue,
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			findings := Detect(input)
			f, ok := firstFinding(findings, patterns.PatternAWSSecretAccessKey)
			if !ok {
				t.Fatalf("expected aws secret finding, got %+v", findings)
			}
			if f.Type != patterns.TypeCredential || f.Severity != patterns.SeverityBlock {
				t.Fatalf("unexpected type/severity: %+v", f)
			}
			if f.Position < 0 || f.End <= f.Position {
				t.Fatalf("invalid finding positions: %+v", f)
			}
			if f.Matched == "" {
				t.Fatalf("expected matched text")
			}
		})
	}
}

func TestDetectAWSAccessKeyID(t *testing.T) {
	findings := Detect("AWS_ACCESS_KEY_ID=" + awsAccessKeyID)
	if _, ok := firstFinding(findings, patterns.PatternAWSAccessKeyID); !ok {
		t.Fatalf("expected aws access key id finding, got %+v", findings)
	}
}

func TestDetectAWSSessionToken(t *testing.T) {
	findings := Detect("AWS_SESSION_TOKEN=" + awsSessionToken)
	if _, ok := firstFinding(findings, patterns.PatternAWSSessionToken); !ok {
		t.Fatalf("expected aws session token finding, got %+v", findings)
	}
}

func TestDetectAWSSecretRejectsInvalidLength(t *testing.T) {
	findings := Detect("AWS_SECRET_ACCESS_KEY=short")
	if _, ok := firstFinding(findings, patterns.PatternAWSSecretAccessKey); ok {
		t.Fatalf("unexpected aws secret finding: %+v", findings)
	}
}

func TestDetectAWSSecretRequiresContext(t *testing.T) {
	findings := Detect(awsSecretValue)
	if _, ok := firstFinding(findings, patterns.PatternAWSSecretAccessKey); ok {
		t.Fatalf("unexpected context-free aws secret finding: %+v", findings)
	}
}

func TestDetectGenericContextualSecrets(t *testing.T) {
	tests := []struct {
		input   string
		pattern string
	}{
		{"CLIENT_SECRET=AbCdEfGhIjKlMnOpQrSt12345!@#$", patterns.PatternGenericSecretAssignment},
		{"API_TOKEN=tok_AbCdEfGhIjKlMnOpQrSt123456789", patterns.PatternGenericTokenAssignment},
		{"private_key=-----BEGIN-PRIVATE-KEY-AbCdEfGh123456", patterns.PatternGenericPrivateKeyAssignment},
		{"service_api_key=key_AbCdEfGhIjKlMnOpQrSt123456", patterns.PatternGenericAPIKeyAssignment},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			findings := Detect(tc.input)
			if _, ok := firstFinding(findings, tc.pattern); !ok {
				t.Fatalf("expected %s, got %+v", tc.pattern, findings)
			}
		})
	}
}

func TestDetectGenericRejectsLowEntropyValues(t *testing.T) {
	findings := Detect("CLIENT_SECRET=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, ok := firstFinding(findings, patterns.PatternGenericSecretAssignment); ok {
		t.Fatalf("unexpected low-entropy generic secret finding: %+v", findings)
	}
}

func TestDetectGenericDoesNotDuplicateAWSFindings(t *testing.T) {
	findings := Detect("AWS_SECRET_ACCESS_KEY=" + awsSecretValue)
	if countFindings(findings, patterns.PatternAWSSecretAccessKey) != 1 {
		t.Fatalf("expected one aws secret finding, got %+v", findings)
	}
	if countFindings(findings, patterns.PatternGenericSecretAssignment) != 0 {
		t.Fatalf("did not expect generic duplicate, got %+v", findings)
	}
}

func firstFinding(findings []Finding, pattern string) (Finding, bool) {
	for _, f := range findings {
		if f.Pattern == pattern {
			return f, true
		}
	}
	return Finding{}, false
}

func countFindings(findings []Finding, pattern string) int {
	count := 0
	for _, f := range findings {
		if f.Pattern == pattern {
			count++
		}
	}
	return count
}
