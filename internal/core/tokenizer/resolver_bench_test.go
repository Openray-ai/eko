package tokenizer

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
)

// --- helpers ---

// populateVaultWithTokens creates a vault, generates token count email tokens via the tokenizer,
// and returns the vault, the list of (original, token) pairs, and a body containing the tokens.
func populateVaultWithTokens(vm *VaultManager, tok *Tokenizer, count int) (*Vault, []string, []string) {
	id := GenerateSessionID()
	vault, _ := vm.GetOrCreate(id)

	originals := make([]string, count)
	tokens := make([]string, count)

	for i := 0; i < count; i++ {
		email := fmt.Sprintf("user%d@domain%d.com", i, i)
		v := detector.Violation{
			Pattern:  patterns.PatternEmail,
			Matched:  email,
			Type:     patterns.TypePII,
			Severity: patterns.SeverityBlock,
		}
		token, _ := tok.Generate(v, vault)
		originals[i] = email
		tokens[i] = token
	}
	return vault, originals, tokens
}

// buildBodyWithTokens creates a body string of approximately baseSize bytes
// with the given tokens scattered throughout.
func buildBodyWithTokens(baseSize int, tokens []string) []byte {
	const phrase = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. "
	reps := baseSize/len(phrase) + 2
	filler := strings.Repeat(phrase, reps)
	if len(filler) > baseSize {
		filler = filler[:baseSize]
	}

	if len(tokens) == 0 {
		return []byte(filler)
	}

	spacing := len(filler) / (len(tokens) + 1)
	var b strings.Builder
	b.Grow(baseSize + len(tokens)*30)

	pos := 0
	for _, tok := range tokens {
		end := pos + spacing
		if end > len(filler) {
			end = len(filler)
		}
		b.WriteString(filler[pos:end])
		b.WriteString(" ")
		b.WriteString(tok)
		b.WriteString(" ")
		pos = end
	}
	if pos < len(filler) {
		b.WriteString(filler[pos:])
	}
	return []byte(b.String())
}

// --- By token count ---

func BenchmarkResolver_ByTokenCount(b *testing.B) {
	for _, tokenCount := range []int{1, 10, 50, 100, 500, 1000} {
		b.Run(fmt.Sprintf("tokens_%d", tokenCount), func(b *testing.B) {
			b.ReportAllocs()
			vm := NewVaultManager(time.Hour)
			defer vm.Stop()
			tok := NewTokenizer()

			vault, _, tokens := populateVaultWithTokens(vm, tok, tokenCount)

			body := buildBodyWithTokens(10_000, tokens)
			sessionID := vault.sessionID

			resolver := NewResolver(vm)
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, _ := resolver.ResolveResponse(body, sessionID)
				runtime.KeepAlive(result)
			}
		})
	}
}

// --- By body size ---

func BenchmarkResolver_ByBodySize(b *testing.B) {
	for _, bodySize := range []int{1_000, 10_000, 100_000, 1_000_000} {
		b.Run(fmt.Sprintf("body_%dB", bodySize), func(b *testing.B) {
			b.ReportAllocs()
			vm := NewVaultManager(time.Hour)
			defer vm.Stop()
			tok := NewTokenizer()

			vault, _, tokens := populateVaultWithTokens(vm, tok, 10)

			body := buildBodyWithTokens(bodySize, tokens)
			sessionID := vault.sessionID

			resolver := NewResolver(vm)
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, _ := resolver.ResolveResponse(body, sessionID)
				runtime.KeepAlive(result)
			}
		})
	}
}

// --- Empty / no-op path ---

func BenchmarkResolver_EmptyBody(b *testing.B) {
	b.ReportAllocs()
	vm := NewVaultManager(time.Hour)
	defer vm.Stop()

	id := GenerateSessionID()
	_, _ = vm.GetOrCreate(id)

	resolver := NewResolver(vm)
	body := []byte{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, _ := resolver.ResolveResponse(body, id)
		runtime.KeepAlive(result)
	}
}

func BenchmarkResolver_NoTokens(b *testing.B) {
	b.ReportAllocs()
	vm := NewVaultManager(time.Hour)
	defer vm.Stop()

	id := GenerateSessionID()
	_, _ = vm.GetOrCreate(id)

	resolver := NewResolver(vm)
	body := []byte(strings.Repeat("no tokens here. ", 1000))
	b.SetBytes(int64(len(body)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, _ := resolver.ResolveResponse(body, id)
		runtime.KeepAlive(result)
	}
}
