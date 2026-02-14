package sanitizer

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/tokenizer"
)

// --- helpers ---

// benchDetector creates a Detector loaded with representative patterns.
func benchDetector() *detector.Detector {
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

	d := detector.New()
	compiled := make([]*patterns.CompiledPattern, len(raw))
	for i, r := range raw {
		compiled[i] = &patterns.CompiledPattern{
			Name:     r.name,
			Regex:    regexp.MustCompile(r.regex),
			Type:     r.typ,
			Severity: r.severity,
		}
	}
	d.LoadPatterns(compiled)
	return d
}

func newBenchSanitizer() (*Sanitizer, func()) {
	det := benchDetector()
	tok := tokenizer.NewTokenizer()
	vm := tokenizer.NewVaultManager(time.Hour)
	s := NewWithTokenizer(det, tok, vm, "tokenize")
	return s, vm.Stop
}

func benchSessionID() string {
	return tokenizer.GenerateSessionID()
}

// --- Full pipeline benchmarks ---

func BenchmarkSanitizeWithSession(b *testing.B) {
	cases := []struct {
		name  string
		input string
	}{
		{
			"clean_text",
			strings.Repeat("The quick brown fox jumps over the lazy dog. ", 20),
		},
		{
			"single_email",
			"Please contact alice.smith@company.com for more information about this request.",
		},
		{
			"mixed_pii",
			"User alice@company.com with BVN 12345678901 called from +2348012345678 to inquire about their account status.",
		},
		{
			"dense_emails",
			func() string {
				var sb strings.Builder
				for i := 0; i < 50; i++ {
					sb.WriteString(fmt.Sprintf("Contact user%d@domain%d.com for details. ", i, i))
				}
				return sb.String()
			}(),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			s, cleanup := newBenchSanitizer()
			defer cleanup()

			b.SetBytes(int64(len(tc.input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Use a unique session per iteration to avoid cross-iteration state.
				result, _ := s.SanitizeWithSession(tc.input, benchSessionID())
				runtime.KeepAlive(result)
			}
		})
	}
}

// --- Redact mode benchmark ---

func BenchmarkSanitize_Redact(b *testing.B) {
	det := benchDetector()
	s := New(det)

	input := "Contact alice@company.com and bob@example.org for project details."

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, _ := s.Sanitize(input)
		runtime.KeepAlive(result)
	}
}

// --- By input size ---

func BenchmarkSanitizeWithSession_ByInputSize(b *testing.B) {
	for _, size := range []int{100, 1_000, 10_000, 100_000} {
		b.Run(fmt.Sprintf("size_%dB", size), func(b *testing.B) {
			b.ReportAllocs()
			s, cleanup := newBenchSanitizer()
			defer cleanup()

			// Build input: mostly clean text with a few emails.
			const phrase = "The quick brown fox jumps over the lazy dog. "
			base := strings.Repeat(phrase, size/len(phrase)+2)
			input := base[:size] + " alice@company.com bob@example.org "

			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, _ := s.SanitizeWithSession(input, benchSessionID())
				runtime.KeepAlive(result)
			}
		})
	}
}

// --- Concurrent sanitization ---

func BenchmarkSanitizeWithSession_Concurrent(b *testing.B) {
	b.ReportAllocs()
	s, cleanup := newBenchSanitizer()
	defer cleanup()

	input := "User alice@company.com with BVN 12345678901 needs support."
	b.SetBytes(int64(len(input)))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, _ := s.SanitizeWithSession(input, benchSessionID())
			runtime.KeepAlive(result)
		}
	})
}
