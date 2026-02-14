package tokenizer

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
)

// --- helpers ---

func benchViolation(pattern, matched, typ, severity string) detector.Violation {
	return detector.Violation{
		Pattern:  pattern,
		Matched:  matched,
		Type:     typ,
		Severity: severity,
	}
}

// --- Per-generator benchmarks ---

func BenchmarkTokenizer_Generate(b *testing.B) {
	tok := NewTokenizer()

	cases := []struct {
		name      string
		violation detector.Violation
	}{
		{
			"email",
			benchViolation(patterns.PatternEmail, "john.doe@gmail.com", patterns.TypePII, patterns.SeverityBlock),
		},
		{
			"nigerian_phone",
			benchViolation(patterns.PatternNigerianPhone, "+2348012345678", patterns.TypePII, patterns.SeverityWarn),
		},
		{
			"kenyan_phone",
			benchViolation(patterns.PatternKenyanPhone, "+254712345678", patterns.TypePII, patterns.SeverityWarn),
		},
		{
			"nigerian_bvn",
			benchViolation(patterns.PatternNigerianBVN, "12345678901", patterns.TypePII, patterns.SeverityBlock),
		},
		{
			"nigerian_account",
			benchViolation(patterns.PatternNigerianAccount, "0123456789", patterns.TypeFinancial, patterns.SeverityBlock),
		},
		{
			"credit_card",
			benchViolation(patterns.PatternCreditCard, "4111111111111111", patterns.TypeFinancial, patterns.SeverityBlock),
		},
		{
			"iban",
			benchViolation(patterns.PatternIBAN, "DE89370400440532013000", patterns.TypeFinancial, patterns.SeverityBlock),
		},
		{
			"default",
			benchViolation("custom_pattern", "SomeSecretValue123", patterns.TypePII, patterns.SeverityBlock),
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			vm := NewVaultManager(time.Hour, WithMaxVaults(b.N+1))
			defer vm.Stop()

			ids := benchSessionIDs(b.N)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Fresh vault per iteration to avoid cache hits.
				vault, _ := vm.GetOrCreate(ids[i])
				_, _ = tok.Generate(tc.violation, vault)
			}
		})
	}
}

// --- Cache hit path ---

func BenchmarkTokenizer_Generate_CacheHit(b *testing.B) {
	b.ReportAllocs()
	tok := NewTokenizer()
	vm := newBenchVaultManager()
	defer vm.Stop()

	vault, _ := vm.GetOrCreate(GenerateSessionID())
	v := benchViolation(patterns.PatternEmail, "john@gmail.com", patterns.TypePII, patterns.SeverityBlock)
	_, _ = tok.Generate(v, vault)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tok.Generate(v, vault)
	}
}

// --- emailSubTokens benchmark ---

func BenchmarkEmailSubTokens(b *testing.B) {
	b.ReportAllocs()

	originals := []string{
		"john@gmail.com",
		"alice.wonderland@company.co.uk",
		"short@a.io",
	}
	tokens := []string{
		"user_1@examp.eac",
		"user_aaaaaaaaaaaa@exampl.eaaaa",
		"user1@e.xa",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % len(originals)
		subs := emailSubTokens(originals[idx], tokens[idx])
		runtime.KeepAlive(subs)
	}
}

// --- Concurrent generation ---

func BenchmarkTokenizer_Generate_Concurrent(b *testing.B) {
	for _, goroutines := range []int{1, 8, 64} {
		b.Run(fmt.Sprintf("goroutines_%d", goroutines), func(b *testing.B) {
			b.ReportAllocs()
			tok := NewTokenizer()
			vm := newBenchVaultManager()
			defer vm.Stop()

			vault, _ := vm.GetOrCreate(GenerateSessionID())

			b.SetParallelism(goroutines)
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				localID := 0
				for pb.Next() {
					localID++
					email := fmt.Sprintf("user%d@domain%d.com", localID, localID)
					v := benchViolation(patterns.PatternEmail, email, patterns.TypePII, patterns.SeverityBlock)
					_, _ = tok.Generate(v, vault)
				}
			})
		})
	}
}

// --- Multi-email tokenization (sub-token overhead) ---

func BenchmarkTokenizer_Generate_EmailWithSubTokens(b *testing.B) {
	b.ReportAllocs()
	tok := NewTokenizer()
	vm := NewVaultManager(time.Hour, WithMaxVaults(b.N+1))
	defer vm.Stop()

	ids := benchSessionIDs(b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vault, _ := vm.GetOrCreate(ids[i])
		v := benchViolation(patterns.PatternEmail, "benchmark@testing.org", patterns.TypePII, patterns.SeverityBlock)
		_, _ = tok.Generate(v, vault)
	}
}

// --- Credential rejection (fast path) ---

func BenchmarkTokenizer_Generate_CredentialRejection(b *testing.B) {
	b.ReportAllocs()
	tok := NewTokenizer()
	vm := newBenchVaultManager()
	defer vm.Stop()

	vault, _ := vm.GetOrCreate(GenerateSessionID())
	v := benchViolation(patterns.PatternOpenAIAPIKey, "sk-abc123", patterns.TypeCredential, patterns.SeverityBlock)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tok.Generate(v, vault)
	}
}

// --- Generator selection benchmark ---

func BenchmarkTokenizer_GeneratorSelection(b *testing.B) {
	b.ReportAllocs()
	tok := NewTokenizer()
	vm := NewVaultManager(time.Hour, WithMaxVaults(b.N+1))
	defer vm.Stop()

	violations := []detector.Violation{
		benchViolation(patterns.PatternEmail, "a@b.com", patterns.TypePII, patterns.SeverityBlock),
		benchViolation(patterns.PatternNigerianPhone, "+2348012345678", patterns.TypePII, patterns.SeverityWarn),
		benchViolation(patterns.PatternIBAN, "DE89370400440532013000", patterns.TypeFinancial, patterns.SeverityBlock),
		benchViolation(patterns.PatternCreditCard, "4111111111111111", patterns.TypeFinancial, patterns.SeverityBlock),
		benchViolation("unknown_phone", "+1234567890", patterns.TypePII, patterns.SeverityBlock),
	}

	ids := benchSessionIDs(b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vault, _ := vm.GetOrCreate(ids[i])
		v := violations[i%len(violations)]
		_, _ = tok.Generate(v, vault)
	}
}
