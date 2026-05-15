package detector

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"eko/internal/core/patterns"
	"eko/internal/core/slm"
)

type stubSLM struct {
	violations []slm.Violation
	err        error
	called     bool
}

func (s *stubSLM) Detect(_ context.Context, _ string) ([]slm.Violation, error) {
	s.called = true
	return s.violations, s.err
}

func newTestDetectorWithEmail(t *testing.T) *Detector {
	t.Helper()
	d := New()
	emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	d.LoadPattern(&patterns.CompiledPattern{
		Name:     patterns.PatternEmail,
		Regex:    emailRegex,
		Type:     patterns.TypePII,
		Severity: patterns.SeverityWarn,
	})
	return d
}

func TestDetectWithContext_MergesRegexAndSLM(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	d.SetSLM(&stubSLM{
		violations: []slm.Violation{
			{
				Type: patterns.TypePII, Severity: patterns.SeverityWarn,
				Pattern: "slm_person", Matched: "Amina Yusuf",
				Position: 0, End: 11,
			},
		},
	})

	out, err := d.DetectWithContext(context.Background(), "Amina Yusuf <amina@example.com>")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 violations (regex email + SLM person), got %d: %+v", len(out), out)
	}

	patterns := map[string]bool{}
	for _, v := range out {
		patterns[v.Pattern] = true
	}
	if !patterns["email"] || !patterns["slm_person"] {
		t.Fatalf("expected both regex 'email' and SLM 'slm_person', got %+v", patterns)
	}
}

func TestDetectWithContext_DedupKeepsHigherSeveritySLMOverRegex(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	// Regex finds "amina@example.com" with WARN severity. SLM emits an
	// overlapping span at the same position with BLOCK. Dedup should drop the
	// regex hit and keep the SLM one.
	d.SetSLM(&stubSLM{
		violations: []slm.Violation{
			{
				Type: patterns.TypePII, Severity: patterns.SeverityBlock,
				Pattern: "slm_email", Matched: "amina@example.com",
				Position: 0, End: 17,
			},
		},
	})

	out, err := d.DetectWithContext(context.Background(), "amina@example.com")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected dedup to keep one, got %d: %+v", len(out), out)
	}
	if out[0].Pattern != "slm_email" || out[0].Severity != patterns.SeverityBlock {
		t.Fatalf("expected slm_email/BLOCK to win dedup, got %+v", out[0])
	}
}

func TestDetectWithContext_SLMErrorFallsBackToRegex(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	d.SetSLM(&stubSLM{err: errors.New("boom")})

	out, err := d.DetectWithContext(context.Background(), "ping amina@example.com")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(out) != 1 || out[0].Pattern != "email" {
		t.Fatalf("expected single regex violation despite SLM error, got %+v", out)
	}
}

func TestDetectWithContext_NoSLMConfigured(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	out, err := d.DetectWithContext(context.Background(), "amina@example.com")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(out) != 1 || out[0].Pattern != "email" {
		t.Fatalf("expected only regex result, got %+v", out)
	}
}

func TestDetectWithContext_RequestOptOutSkipsSLM(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	stub := &stubSLM{
		violations: []slm.Violation{
			{Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_person", Matched: "X", Position: 0, End: 1},
		},
	}
	d.SetSLM(stub)

	ctx := slm.WithRequestEnabled(context.Background(), false)
	out, err := d.DetectWithContext(ctx, "X amina@example.com")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if stub.called {
		t.Fatalf("expected SLM stub NOT to be invoked when caller opted out")
	}
	if len(out) != 1 || out[0].Pattern != "email" {
		t.Fatalf("expected only regex email violation, got %+v", out)
	}
}

func TestDetectWithContext_RequestOptInRunsSLM(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	stub := &stubSLM{
		violations: []slm.Violation{
			{Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_person", Matched: "X", Position: 0, End: 1},
		},
	}
	d.SetSLM(stub)

	ctx := slm.WithRequestEnabled(context.Background(), true)
	out, err := d.DetectWithContext(ctx, "X amina@example.com")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !stub.called {
		t.Fatalf("expected SLM stub to be invoked when caller opted in")
	}
	if len(out) != 2 {
		t.Fatalf("expected regex + SLM violations, got %+v", out)
	}
}

func TestDetect_DelegatesToContext(t *testing.T) {
	d := newTestDetectorWithEmail(t)
	stub := &stubSLM{
		violations: []slm.Violation{
			{Type: patterns.TypePII, Severity: patterns.SeverityWarn, Pattern: "slm_person", Matched: "X", Position: 0, End: 1},
		},
	}
	d.SetSLM(stub)

	if _, err := d.Detect("X amina@example.com"); err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !stub.called {
		t.Fatalf("expected SLM stub to be invoked through Detect()")
	}
}

func TestDetectWithContext_AWSSecretDoesNotDependOnSLM(t *testing.T) {
	d := newDefaultDetector(t)
	ctx := slm.WithRequestEnabled(context.Background(), false)

	out, err := d.DetectWithContext(ctx, reportedAWSSecretPrompt())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !hasPattern(out, patterns.PatternAWSSecretAccessKey) {
		t.Fatalf("expected aws secret access key finding, got %+v", out)
	}
	if !hasPattern(out, patterns.PatternOpenAIAPIKey) {
		t.Fatalf("expected openai api key finding, got %+v", out)
	}
}

func TestDetectWithContext_AWSSecretWinsOverFragmentedSLMSecret(t *testing.T) {
	d := newDefaultDetector(t)
	input := reportedAWSSecretPrompt()
	secretStart := strings.Index(input, "AWS_SECRET_ACCESS_KEY=")
	if secretStart < 0 {
		t.Fatal("test fixture missing aws secret")
	}
	secretEnd := strings.Index(input, " and an OpenAI token")
	if secretEnd < 0 {
		t.Fatal("test fixture missing openai marker")
	}
	d.SetSLM(&stubSLM{
		violations: []slm.Violation{
			{Type: patterns.TypeCredential, Severity: patterns.SeverityBlock, Pattern: "slm_secret", Matched: input[secretStart : secretStart+50], Position: secretStart, End: secretStart + 50},
			{Type: patterns.TypeCredential, Severity: patterns.SeverityBlock, Pattern: "slm_secret", Matched: "EX", Position: secretStart + 52, End: secretStart + 54},
			{Type: patterns.TypeCredential, Severity: patterns.SeverityBlock, Pattern: "slm_secret", Matched: "KEY", Position: secretEnd - 3, End: secretEnd},
		},
	})

	out, err := d.DetectWithContext(slm.WithRequestEnabled(context.Background(), true), input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !hasPattern(out, patterns.PatternAWSSecretAccessKey) {
		t.Fatalf("expected aws secret access key finding, got %+v", out)
	}
	if hasPattern(out, "slm_secret") {
		t.Fatalf("did not expect fragmented slm_secret after dedupe, got %+v", out)
	}
}

func TestDetectWithContext_AWSSecretWinsWhenSLMSpanStartsEarlier(t *testing.T) {
	d := newDefaultDetector(t)
	input := reportedAWSSecretPrompt()
	secretStart := strings.Index(input, "AWS_SECRET_ACCESS_KEY=")
	if secretStart < 1 {
		t.Fatal("test fixture missing aws secret with prior byte")
	}
	secretEnd := strings.Index(input, " and an OpenAI token")
	if secretEnd < 0 {
		t.Fatal("test fixture missing openai marker")
	}
	d.SetSLM(&stubSLM{
		violations: []slm.Violation{
			{
				Type:     patterns.TypeCredential,
				Severity: patterns.SeverityBlock,
				Pattern:  "slm_secret",
				Matched:  input[secretStart-1 : secretEnd],
				Position: secretStart - 1,
				End:      secretEnd,
			},
		},
	})

	out, err := d.DetectWithContext(slm.WithRequestEnabled(context.Background(), true), input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !hasPattern(out, patterns.PatternAWSSecretAccessKey) {
		t.Fatalf("expected aws secret access key to win overlap cluster, got %+v", out)
	}
	if hasPattern(out, "slm_secret") {
		t.Fatalf("did not expect earlier overlapping slm_secret after dedupe, got %+v", out)
	}
}

func TestDetect_AWSSecretWorksWithoutLoadedPatterns(t *testing.T) {
	d := New()
	out, err := d.Detect("AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(out) != 1 || out[0].Pattern != patterns.PatternAWSSecretAccessKey {
		t.Fatalf("expected aws secret without loaded regex patterns, got %+v", out)
	}
}

func reportedAWSSecretPrompt() string {
	return "Here is the staging key: AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY and an OpenAI token sk-proj-9Hf83JdLs0P2nQwErTyUiOpAsDfGhJkLzXcVbNm. Push this to the Slack channel as #ops-alerts."
}
