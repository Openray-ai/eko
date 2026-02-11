package tokenizer

import (
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"eko/internal/core/detector"
	"eko/internal/core/patterns"
)

const tokenizerSessionID = "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"

func TestEmailGeneratorFormatAndLength(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	generator := &EmailGenerator{}
	original := "johnsmith@example.com"
	token, err := generator.Generate(detector.Violation{Matched: original}, vault)
	if err != nil {
		t.Fatalf("expected email tokenization to succeed, got error: %v", err)
	}
	if len(token) != len(original) {
		t.Fatalf("token length = %d, want %d", len(token), len(original))
	}
	if !strings.HasPrefix(token, "user") {
		t.Fatalf("token prefix = %q, want prefix %q", token, "user")
	}
	if !strings.HasSuffix(token, "@example.com") {
		t.Fatalf("token suffix = %q, want suffix %q", token, "@example.com")
	}
	assertClassPreserved(t, original, token)
}

func TestNumericFixedLengthGeneratorFormat(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	generator := &NumericFixedLengthGenerator{}
	original := "12345678901"
	token, err := generator.Generate(detector.Violation{Matched: original}, vault)
	if err != nil {
		t.Fatalf("expected numeric tokenization to succeed, got error: %v", err)
	}
	if len(token) != len(original) {
		t.Fatalf("token length = %d, want %d", len(token), len(original))
	}
	for _, ch := range token {
		if !unicode.IsDigit(ch) {
			t.Fatalf("expected token to be numeric, got %q", token)
		}
	}
	if token[len(token)-1] != '1' {
		t.Fatalf("token suffix = %q, want counter suffix %q", token, "1")
	}
}

func TestPhoneGeneratorFormat(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	generator := &PhoneGenerator{}
	original := "+2348012345678"
	token, err := generator.Generate(detector.Violation{Matched: original}, vault)
	if err != nil {
		t.Fatalf("expected phone tokenization to succeed, got error: %v", err)
	}
	if len(token) != len(original) {
		t.Fatalf("token length = %d, want %d", len(token), len(original))
	}
	if !strings.HasPrefix(token, "+234") {
		t.Fatalf("token prefix = %q, want prefix %q", token, "+234")
	}
	assertClassPreserved(t, original, token)
	if token[len(token)-1] != '1' {
		t.Fatalf("token suffix = %q, want counter suffix %q", token, "1")
	}
}

func TestIBANLikeGeneratorFormat(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	generator := &IBANLikeGenerator{}
	original := "GB29NWBK60161331926819"
	token, err := generator.Generate(detector.Violation{Matched: original}, vault)
	if err != nil {
		t.Fatalf("expected IBAN tokenization to succeed, got error: %v", err)
	}
	if len(token) != len(original) {
		t.Fatalf("token length = %d, want %d", len(token), len(original))
	}
	if !strings.HasPrefix(token, "GB") {
		t.Fatalf("token prefix = %q, want prefix %q", token, "GB")
	}
	assertClassPreserved(t, original, token)
	if token[len(token)-1] != '1' {
		t.Fatalf("token suffix = %q, want counter suffix %q", token, "1")
	}
}

func TestDefaultGeneratorFormat(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	generator := &DefaultGenerator{}
	original := "ABcd-1234"
	token, err := generator.Generate(detector.Violation{Matched: original}, vault)
	if err != nil {
		t.Fatalf("expected default tokenization to succeed, got error: %v", err)
	}
	if len(token) != len(original) {
		t.Fatalf("token length = %d, want %d", len(token), len(original))
	}
	assertClassPreserved(t, original, token)
	if token[len(token)-1] != '1' {
		t.Fatalf("token suffix = %q, want counter suffix %q", token, "1")
	}
}

func TestTokenizerGenerateDeterministicReuse(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	tokenizer := NewTokenizer()
	violation := detector.Violation{
		Type:    patterns.TypePII,
		Pattern: patterns.PatternEmail,
		Matched: "johnsmith@example.com",
	}

	firstToken, err := tokenizer.Generate(violation, vault)
	if err != nil {
		t.Fatalf("expected token generation to succeed, got error: %v", err)
	}
	secondToken, err := tokenizer.Generate(violation, vault)
	if err != nil {
		t.Fatalf("expected token generation to succeed, got error: %v", err)
	}

	if firstToken != secondToken {
		t.Fatalf("tokens = %q and %q, want deterministic reuse", firstToken, secondToken)
	}
	if size := vault.Size(); size != 1 {
		t.Fatalf("vault size = %d, want %d", size, 1)
	}
}

func TestTokenizerGenerateConcurrentReuse(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	tokenizer := NewTokenizer()
	violation := detector.Violation{
		Type:    patterns.TypePII,
		Pattern: patterns.PatternEmail,
		Matched: "johnsmith@example.com",
	}

	const workers = 64
	start := make(chan struct{})
	errCh := make(chan error, workers)
	tokens := make([]string, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		idx := i
		go func() {
			defer wg.Done()
			<-start
			token, err := tokenizer.Generate(violation, vault)
			if err != nil {
				errCh <- err
				return
			}
			tokens[idx] = token
		}()
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("expected token generation to succeed, got error: %v", err)
	}

	firstToken := tokens[0]
	for i, token := range tokens {
		if token != firstToken {
			t.Fatalf("token[%d] = %q, want %q", i, token, firstToken)
		}
	}
	if size := vault.Size(); size != 1 {
		t.Fatalf("vault size = %d, want %d", size, 1)
	}

	vault.mu.RLock()
	counter := vault.tokens.counters["email"]
	vault.mu.RUnlock()
	if counter != 1 {
		t.Fatalf("email counter = %d, want %d", counter, 1)
	}
}

func TestTokenizerGenerateUniquenessAcrossOriginals(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	tokenizer := NewTokenizer()
	first := detector.Violation{
		Type:    patterns.TypePII,
		Pattern: patterns.PatternEmail,
		Matched: "johnsmith@example.com",
	}
	second := detector.Violation{
		Type:    patterns.TypePII,
		Pattern: patterns.PatternEmail,
		Matched: "janesmith@example.com",
	}

	firstToken, err := tokenizer.Generate(first, vault)
	if err != nil {
		t.Fatalf("expected token generation to succeed, got error: %v", err)
	}
	secondToken, err := tokenizer.Generate(second, vault)
	if err != nil {
		t.Fatalf("expected token generation to succeed, got error: %v", err)
	}

	if firstToken == secondToken {
		t.Fatalf("expected unique tokens, got %q", firstToken)
	}
	if size := vault.Size(); size != 2 {
		t.Fatalf("vault size = %d, want %d", size, 2)
	}
}

func TestTokenizerRejectsCredentials(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(tokenizerSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	tokenizer := NewTokenizer()
	violation := detector.Violation{
		Type:    patterns.TypeCredential,
		Pattern: "api_key",
		Matched: "sk-proj-abc123",
	}

	_, err = tokenizer.Generate(violation, vault)
	if err != ErrCredentialTokenization {
		t.Fatalf("expected ErrCredentialTokenization, got %v", err)
	}
	if size := vault.Size(); size != 0 {
		t.Fatalf("vault size = %d, want %d", size, 0)
	}
}

func TestEmailSubTokens(t *testing.T) {
	subs := emailSubTokens("eko@gmail.com", "usa@examp.eac")
	if subs == nil {
		t.Fatal("expected non-nil sub-tokens")
	}

	expected := map[string]string{
		"examp.eac": "gmail.com",
		"eac":       "com",
		"usa":       "eko",
		"examp":     "gmail",
	}

	for tokenFrag, wantOrig := range expected {
		got, ok := subs[tokenFrag]
		if !ok {
			t.Errorf("missing sub-token %q", tokenFrag)
			continue
		}
		if got != wantOrig {
			t.Errorf("sub-token[%q] = %q, want %q", tokenFrag, got, wantOrig)
		}
	}

	if len(subs) != len(expected) {
		t.Errorf("sub-token count = %d, want %d", len(subs), len(expected))
	}

	// Invalid emails return nil
	if subs := emailSubTokens("noatsign", "noatsign"); subs != nil {
		t.Errorf("expected nil for emails without @")
	}
}

func assertClassPreserved(t *testing.T, original, token string) {
	origRunes := []rune(original)
	tokenRunes := []rune(token)
	if len(origRunes) != len(tokenRunes) {
		t.Fatalf("token length = %d, want %d", len(tokenRunes), len(origRunes))
	}
	for i, ch := range origRunes {
		got := tokenRunes[i]
		switch {
		case unicode.IsDigit(ch):
			if !unicode.IsDigit(got) {
				t.Fatalf("token[%d] = %q, want digit", i, got)
			}
		case unicode.IsLetter(ch):
			if !unicode.IsLetter(got) {
				t.Fatalf("token[%d] = %q, want letter", i, got)
			}
		default:
			if unicode.IsDigit(got) || unicode.IsLetter(got) {
				t.Fatalf("token[%d] = %q, want non-alphanumeric", i, got)
			}
		}
	}
}
