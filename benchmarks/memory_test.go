package benchmarks

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
	"eko/internal/core/sanitizer"
	"eko/internal/core/tokenizer"
)

// --- helpers ---

func heapAllocMB() float64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1024 * 1024)
}

func heapAllocBytes() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

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
		{patterns.PatternCreditCard, `\b[0-9]{13,19}\b`, patterns.TypeFinancial, patterns.SeverityBlock},
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

// --- Memory growth: vaults ---

func TestMemoryVaultGrowth(t *testing.T) {
	checkpoints := []int{100, 500, 1_000, 2_000, 5_000, 10_000}
	tokensPerVault := 10

	vm := tokenizer.NewVaultManager(time.Hour, tokenizer.WithMaxVaults(checkpoints[len(checkpoints)-1]+1))
	defer vm.Stop()
	tok := tokenizer.NewTokenizer()

	baseline := heapAllocMB()
	t.Logf("Baseline heap: %.2f MB", baseline)

	vaultIdx := 0
	for _, target := range checkpoints {
		for vaultIdx < target {
			id := tokenizer.GenerateSessionID()
			vault, err := vm.GetOrCreate(id)
			if err != nil {
				t.Fatalf("GetOrCreate failed at vault %d: %v", vaultIdx, err)
			}
			for j := 0; j < tokensPerVault; j++ {
				email := fmt.Sprintf("user%d_%d@domain%d.com", vaultIdx, j, vaultIdx)
				v := detector.Violation{
					Pattern:  patterns.PatternEmail,
					Matched:  email,
					Type:     patterns.TypePII,
					Severity: patterns.SeverityBlock,
				}
				_, err := tok.Generate(v, vault)
				if err != nil {
					t.Fatalf("Generate failed: %v", err)
				}
			}
			vaultIdx++
		}

		heap := heapAllocMB()
		totalTokens := target * tokensPerVault
		bytesPerToken := float64(0)
		if totalTokens > 0 {
			bytesPerToken = (heap - baseline) * 1024 * 1024 / float64(totalTokens)
		}
		t.Logf("Vaults: %5d | Tokens: %7d | Heap: %8.2f MB | delta: %8.2f MB | ~%.0f bytes/token",
			target, totalTokens, heap, heap-baseline, bytesPerToken)
	}
}

// --- Memory growth: tokens in single vault ---

func TestMemoryTokenGrowth(t *testing.T) {
	checkpoints := []int{100, 1_000, 5_000, 10_000, 50_000, 100_000}

	vm := tokenizer.NewVaultManager(time.Hour, tokenizer.WithMaxTokensPerVault(checkpoints[len(checkpoints)-1]+1))
	defer vm.Stop()
	tok := tokenizer.NewTokenizer()

	id := tokenizer.GenerateSessionID()
	vault, err := vm.GetOrCreate(id)
	if err != nil {
		t.Fatal(err)
	}

	baseline := heapAllocMB()
	t.Logf("Baseline heap: %.2f MB", baseline)

	tokenIdx := 0
	for _, target := range checkpoints {
		for tokenIdx < target {
			// Use zero-padded format to ensure enough digit positions for the email generator counter.
			email := fmt.Sprintf("user%06d@domain%06d.com", tokenIdx, tokenIdx)
			v := detector.Violation{
				Pattern:  patterns.PatternEmail,
				Matched:  email,
				Type:     patterns.TypePII,
				Severity: patterns.SeverityBlock,
			}
			_, err := tok.Generate(v, vault)
			if err != nil {
				t.Fatalf("Generate failed at token %d: %v", tokenIdx, err)
			}
			tokenIdx++
		}

		heap := heapAllocMB()
		bytesPerToken := float64(0)
		if target > 0 {
			bytesPerToken = (heap - baseline) * 1024 * 1024 / float64(target)
		}
		t.Logf("Tokens: %6d | Heap: %8.2f MB | delta: %8.2f MB | ~%.0f bytes/token",
			target, heap, heap-baseline, bytesPerToken)
	}
}

// --- Cleanup recovery ---

func TestMemoryCleanupRecovery(t *testing.T) {
	vaultCount := 5_000
	tokensPerVault := 10

	vm := tokenizer.NewVaultManager(50*time.Millisecond, tokenizer.WithMaxVaults(vaultCount+1))
	defer vm.Stop()
	tok := tokenizer.NewTokenizer()

	baseline := heapAllocMB()
	t.Logf("Baseline heap: %.2f MB", baseline)

	for i := 0; i < vaultCount; i++ {
		id := tokenizer.GenerateSessionID()
		vault, err := vm.GetOrCreate(id)
		if err != nil {
			t.Fatalf("GetOrCreate failed: %v", err)
		}
		for j := 0; j < tokensPerVault; j++ {
			email := fmt.Sprintf("user%d_%d@domain%d.com", i, j, i)
			v := detector.Violation{
				Pattern:  patterns.PatternEmail,
				Matched:  email,
				Type:     patterns.TypePII,
				Severity: patterns.SeverityBlock,
			}
			_, _ = tok.Generate(v, vault)
		}
	}

	heapBefore := heapAllocMB()
	t.Logf("After filling %d vaults: %.2f MB (delta: %.2f MB)", vaultCount, heapBefore, heapBefore-baseline)

	// Wait for TTL expiry and force cleanup.
	time.Sleep(100 * time.Millisecond)
	vm.Cleanup()

	heapAfter := heapAllocMB()
	recovery := float64(0)
	if heapBefore-baseline > 0 {
		recovery = (heapBefore - heapAfter) / (heapBefore - baseline) * 100
	}
	t.Logf("After cleanup+GC: %.2f MB (recovered: %.1f%%)", heapAfter, recovery)
}

// --- Vault matrix ---

func TestMemoryVaultMatrix(t *testing.T) {
	vaultCounts := []int{100, 1_000, 5_000, 10_000}
	tokenCounts := []int{10, 100, 1_000}

	for _, vc := range vaultCounts {
		for _, tc := range tokenCounts {
			name := fmt.Sprintf("vaults_%d_tokens_%d", vc, tc)
			t.Run(name, func(t *testing.T) {
				vm := tokenizer.NewVaultManager(time.Hour,
					tokenizer.WithMaxVaults(vc+1),
					tokenizer.WithMaxTokensPerVault(tc+1),
				)
				defer vm.Stop()
				tok := tokenizer.NewTokenizer()

				baseline := heapAllocBytes()

				for i := 0; i < vc; i++ {
					id := tokenizer.GenerateSessionID()
					vault, err := vm.GetOrCreate(id)
					if err != nil {
						t.Fatalf("GetOrCreate failed: %v", err)
					}
					for j := 0; j < tc; j++ {
						email := fmt.Sprintf("u%04d_%04d@d%04d.com", i, j, i)
						v := detector.Violation{
							Pattern:  patterns.PatternEmail,
							Matched:  email,
							Type:     patterns.TypePII,
							Severity: patterns.SeverityBlock,
						}
						_, _ = tok.Generate(v, vault)
					}
				}

				heapNow := heapAllocBytes()
				totalTokens := uint64(vc) * uint64(tc)
				delta := heapNow - baseline
				bytesPerToken := uint64(0)
				if totalTokens > 0 {
					bytesPerToken = delta / totalTokens
				}

				// Extrapolate: at 512MB cap with 112MB for runtime, ~400MB usable.
				usableBytes := uint64(400 * 1024 * 1024)
				maxTokens := uint64(0)
				if bytesPerToken > 0 {
					maxTokens = usableBytes / bytesPerToken
				}

				t.Logf("Vaults: %5d | Tokens/vault: %5d | Total: %8d | Heap delta: %8d bytes | ~%d bytes/token | Max@512MB: ~%d tokens",
					vc, tc, totalTokens, delta, bytesPerToken, maxTokens)
			})
		}
	}
}

// --- 512MB boundary probe ---

func TestMemoryCeiling512MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 512MB ceiling test in short mode")
	}

	const targetHeapMB = 400 // Leave 112MB for runtime overhead.
	const targetHeapBytes = targetHeapMB * 1024 * 1024

	vm := tokenizer.NewVaultManager(time.Hour, tokenizer.WithMaxTokensPerVault(10_000_000))
	defer vm.Stop()
	tok := tokenizer.NewTokenizer()

	id := tokenizer.GenerateSessionID()
	vault, err := vm.GetOrCreate(id)
	if err != nil {
		t.Fatal(err)
	}

	baseline := heapAllocBytes()
	t.Logf("Baseline heap: %d bytes (%.2f MB)", baseline, float64(baseline)/(1024*1024))

	count := 0
	checkInterval := 1000
	for {
		email := fmt.Sprintf("user%08d@domain%08d.com", count, count)
		v := detector.Violation{
			Pattern:  patterns.PatternEmail,
			Matched:  email,
			Type:     patterns.TypePII,
			Severity: patterns.SeverityBlock,
		}
		_, err := tok.Generate(v, vault)
		if err != nil {
			t.Logf("Generation stopped at token %d: %v", count, err)
			break
		}
		count++

		if count%checkInterval == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.HeapAlloc >= targetHeapBytes {
				t.Logf("Reached %dMB heap ceiling at %d tokens (HeapAlloc: %.2f MB)",
					targetHeapMB, count, float64(m.HeapAlloc)/(1024*1024))
				break
			}
			if count%100_000 == 0 {
				t.Logf("Progress: %d tokens, heap: %.2f MB", count, float64(m.HeapAlloc)/(1024*1024))
			}
		}
	}

	finalHeap := heapAllocMB()
	t.Logf("Final: %d tokens generated, heap: %.2f MB", count, finalHeap)
}

// --- Stress benchmarks ---

func BenchmarkStress_ConcurrentSessions(b *testing.B) {
	for _, sessions := range []int{10, 50, 100, 500} {
		b.Run(fmt.Sprintf("sessions_%d", sessions), func(b *testing.B) {
			b.ReportAllocs()
			vm := tokenizer.NewVaultManager(time.Hour, tokenizer.WithMaxVaults(sessions*b.N+1))
			defer vm.Stop()

			ids := make([]string, sessions)
			for i := range ids {
				ids[i] = tokenizer.GenerateSessionID()
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				idx := 0
				for pb.Next() {
					_, _ = vm.GetOrCreate(ids[idx%sessions])
					idx++
				}
			})
		})
	}
}

func BenchmarkStress_GrowingVault(b *testing.B) {
	tok := tokenizer.NewTokenizer()

	for _, preload := range []int{100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("preload_%d", preload), func(b *testing.B) {
			b.ReportAllocs()
			vm := tokenizer.NewVaultManager(time.Hour)
			defer vm.Stop()

			id := tokenizer.GenerateSessionID()
			vault, _ := vm.GetOrCreate(id)

			// Pre-fill the vault.
			for i := 0; i < preload; i++ {
				email := fmt.Sprintf("pre%06d@domain%06d.com", i, i)
				v := detector.Violation{
					Pattern:  patterns.PatternEmail,
					Matched:  email,
					Type:     patterns.TypePII,
					Severity: patterns.SeverityBlock,
				}
				_, _ = tok.Generate(v, vault)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				email := fmt.Sprintf("bench%06d@domain%06d.com", preload+i, preload+i)
				v := detector.Violation{
					Pattern:  patterns.PatternEmail,
					Matched:  email,
					Type:     patterns.TypePII,
					Severity: patterns.SeverityBlock,
				}
				_, _ = tok.Generate(v, vault)
			}
		})
	}
}

func BenchmarkStress_LargePayload(b *testing.B) {
	for _, size := range []int{1_000, 10_000, 100_000, 1_000_000} {
		b.Run(fmt.Sprintf("payload_%dB", size), func(b *testing.B) {
			b.ReportAllocs()
			det := benchDetector()
			tok := tokenizer.NewTokenizer()
			vm := tokenizer.NewVaultManager(time.Hour)
			defer vm.Stop()
			s := sanitizer.NewWithTokenizer(det, tok, vm, "tokenize")

			// Build input: clean text with embedded PII.
			const phrase = "The quick brown fox jumps over the lazy dog. "
			base := strings.Repeat(phrase, size/len(phrase)+2)
			input := base[:size] + " alice@company.com bob@example.org 12345678901 "

			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, _ := s.SanitizeWithSession(input, tokenizer.GenerateSessionID())
				runtime.KeepAlive(result)
			}
		})
	}
}
