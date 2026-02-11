package tokenizer

import (
	"testing"
	"time"

	"eko/internal/core/detector"
)

const resolverSessionID = "eko_c7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"

func TestResolveResponseWithSubTokenFragments(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(resolverSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	tok := NewTokenizer()
	violation := detector.Violation{
		Type:    "pii",
		Pattern: "email",
		Matched: "eko@gmail.com",
	}
	token, err := tok.Generate(violation, vault)
	if err != nil {
		t.Fatalf("expected token generation to succeed, got error: %v", err)
	}

	// The LLM might return just the TLD fragment from the token.
	// Find the TLD of the generated token.
	atIdx := -1
	for i, ch := range token {
		if ch == '@' {
			atIdx = i
			break
		}
	}
	if atIdx < 0 {
		t.Fatalf("expected token to contain @, got %q", token)
	}
	tokenDomain := token[atIdx+1:]
	dotIdx := -1
	for i := len(tokenDomain) - 1; i >= 0; i-- {
		if tokenDomain[i] == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx < 0 {
		t.Fatalf("expected token domain to contain ., got %q", tokenDomain)
	}
	tldFragment := tokenDomain[dotIdx+1:]

	// Simulate LLM response containing only the TLD fragment
	body := []byte(`{"root_domain": "` + tldFragment + `"}`)

	resolver := NewResolver(manager)
	resolved, err := resolver.ResolveResponse(body, resolverSessionID)
	if err != nil {
		t.Fatalf("expected resolve to succeed, got error: %v", err)
	}

	expected := `{"root_domain": "com"}`
	if string(resolved) != expected {
		t.Fatalf("resolved = %q, want %q", string(resolved), expected)
	}
}

func TestResolveResponseOrdering(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(resolverSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	// Store tokens where one is a substring of another
	if err := vault.Store("eko@gmail.com", "abc@defgh.ijk"); err != nil {
		t.Fatalf("expected store to succeed, got error: %v", err)
	}
	vault.StoreSubTokens(map[string]string{
		"ijk": "com",
	})

	body := []byte("The email is abc@defgh.ijk and TLD is ijk")

	resolver := NewResolver(manager)
	resolved, err := resolver.ResolveResponse(body, resolverSessionID)
	if err != nil {
		t.Fatalf("expected resolve to succeed, got error: %v", err)
	}

	expected := "The email is eko@gmail.com and TLD is com"
	if string(resolved) != expected {
		t.Fatalf("resolved = %q, want %q", string(resolved), expected)
	}
}
