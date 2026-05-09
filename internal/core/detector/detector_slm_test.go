package detector

import (
	"context"
	"errors"
	"regexp"
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
