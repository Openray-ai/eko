package detector

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"eko/internal/core/patterns"
)

// --- helpers ---

// benchPatterns returns a representative subset of compiled patterns for benchmarking.
func benchPatterns() []*patterns.CompiledPattern {
	raw := []struct {
		name     string
		regex    string
		typ      string
		severity string
	}{
		{patterns.PatternEmail, `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`, patterns.TypePII, patterns.SeverityBlock},
		{patterns.PatternNigerianBVN, `\b[0-9]{11}\b`, patterns.TypePII, patterns.SeverityBlock},
		{patterns.PatternNigerianPhone, `\+234[0-9]{10}|0[789][01][0-9]{8}`, patterns.TypePII, patterns.SeverityWarn},
		{patterns.PatternIBAN, `[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}`, patterns.TypeFinancial, patterns.SeverityBlock},
		{patterns.PatternCreditCard, `\b[0-9]{13,19}\b`, patterns.TypeFinancial, patterns.SeverityBlock},
		{patterns.PatternOpenAIAPIKey, `sk-[a-zA-Z0-9]{48}`, patterns.TypeCredential, patterns.SeverityBlock},
		{patterns.PatternPostgresConn, `postgres://[^\s]+`, patterns.TypeCredential, patterns.SeverityBlock},
		{patterns.PatternJWTToken, `eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`, patterns.TypeCredential, patterns.SeverityBlock},
	}

	compiled := make([]*patterns.CompiledPattern, len(raw))
	for i, r := range raw {
		compiled[i] = &patterns.CompiledPattern{
			Name:     r.name,
			Regex:    regexp.MustCompile(r.regex),
			Type:     r.typ,
			Severity: r.severity,
		}
	}
	return compiled
}

// cleanText returns a string of n bytes with no PII-like content.
func cleanText(n int) string {
	const word = "The quick brown fox jumps over the lazy dog. "
	reps := (n / len(word)) + 1
	return strings.Repeat(word, reps)[:n]
}

// textWithEmails returns clean text of approximately n bytes total with count emails embedded.
func textWithEmails(n, count int) string {
	email := "alice.benchtest@example.com"
	base := cleanText(n)
	if count == 0 {
		return base
	}
	// Distribute emails roughly evenly throughout the text.
	spacing := len(base) / (count + 1)
	var b strings.Builder
	b.Grow(n + count*len(email))
	pos := 0
	for i := 0; i < count; i++ {
		end := pos + spacing
		if end > len(base) {
			end = len(base)
		}
		b.WriteString(base[pos:end])
		b.WriteString(" ")
		b.WriteString(email)
		b.WriteString(" ")
		pos = end
	}
	if pos < len(base) {
		b.WriteString(base[pos:])
	}
	return b.String()
}

// newDetectorWithPatterns creates a Detector loaded with the given patterns.
func newDetectorWithPatterns(pats []*patterns.CompiledPattern) *Detector {
	d := New()
	d.LoadPatterns(pats)
	return d
}

// --- By input size ---

func BenchmarkDetect_ByInputSize(b *testing.B) {
	allPatterns := benchPatterns()
	d := newDetectorWithPatterns(allPatterns)

	for _, size := range []int{100, 1000, 10_000, 100_000, 1_000_000} {
		b.Run(fmt.Sprintf("size_%dB", size), func(b *testing.B) {
			b.ReportAllocs()
			input := cleanText(size)
			b.SetBytes(int64(size))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				violations, _ := d.Detect(input)
				runtime.KeepAlive(violations)
			}
		})
	}
}

// --- By violation density ---

func BenchmarkDetect_ByViolationDensity(b *testing.B) {
	// Use only email pattern to isolate density impact.
	emailPattern := benchPatterns()[0:1]
	d := newDetectorWithPatterns(emailPattern)

	for _, count := range []int{0, 1, 10, 100} {
		b.Run(fmt.Sprintf("emails_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			input := textWithEmails(10_000, count)
			b.SetBytes(int64(len(input)))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				violations, _ := d.Detect(input)
				runtime.KeepAlive(violations)
			}
		})
	}
}

// --- By pattern count ---

func BenchmarkDetect_ByPatternCount(b *testing.B) {
	allPatterns := benchPatterns()
	input := cleanText(10_000)

	for _, count := range []int{1, 5, 8} {
		if count > len(allPatterns) {
			count = len(allPatterns)
		}
		b.Run(fmt.Sprintf("patterns_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			d := newDetectorWithPatterns(allPatterns[:count])

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				violations, _ := d.Detect(input)
				runtime.KeepAlive(violations)
			}
		})
	}

	// Test with duplicated patterns to reach higher counts.
	for _, count := range []int{22, 50} {
		b.Run(fmt.Sprintf("patterns_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			// Create extra patterns by cloning with unique names.
			extra := make([]*patterns.CompiledPattern, count)
			for i := range extra {
				src := allPatterns[i%len(allPatterns)]
				extra[i] = &patterns.CompiledPattern{
					Name:     fmt.Sprintf("%s_%d", src.Name, i),
					Regex:    src.Regex,
					Type:     src.Type,
					Severity: src.Severity,
				}
			}
			d := newDetectorWithPatterns(extra)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				violations, _ := d.Detect(input)
				runtime.KeepAlive(violations)
			}
		})
	}
}

// --- Deduplication cost ---

func BenchmarkDetect_Deduplication(b *testing.B) {
	// Use patterns that produce many overlapping violations.
	// BVN (11 digits) and account (10 digits) overlap heavily on numeric sequences.
	overlapPatterns := []*patterns.CompiledPattern{
		{Name: patterns.PatternNigerianBVN, Regex: regexp.MustCompile(`\b[0-9]{11}\b`), Type: patterns.TypePII, Severity: patterns.SeverityBlock},
		{Name: patterns.PatternNigerianAccount, Regex: regexp.MustCompile(`\b[0-9]{10}\b`), Type: patterns.TypeFinancial, Severity: patterns.SeverityBlock},
		{Name: patterns.PatternCreditCard, Regex: regexp.MustCompile(`\b[0-9]{13,19}\b`), Type: patterns.TypeFinancial, Severity: patterns.SeverityBlock},
	}
	d := newDetectorWithPatterns(overlapPatterns)

	// Input with many numeric sequences that will create overlapping matches.
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString(fmt.Sprintf("Account number is %011d and reference %016d. ", i, i*1000))
	}
	input := sb.String()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		violations, _ := d.Detect(input)
		runtime.KeepAlive(violations)
	}
}

// --- Concurrent detection ---

func BenchmarkDetect_Concurrent(b *testing.B) {
	allPatterns := benchPatterns()
	d := newDetectorWithPatterns(allPatterns)
	input := textWithEmails(10_000, 5)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			violations, _ := d.Detect(input)
			runtime.KeepAlive(violations)
		}
	})
}
