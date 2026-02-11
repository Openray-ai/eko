package tokenizer

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

const (
	sampleSessionID  = "eko_a7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
	sampleSessionID2 = "eko_b7f3b2c1-4d5e-6f7a-8b9c-0d1e2f3a4b5c"
)

func TestVaultStoreAndLookup(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	if err := vault.Store("john@acme.com", "user_1@example.com"); err != nil {
		t.Fatalf("expected store to succeed, got error: %v", err)
	}

	token, ok := vault.GetToken("john@acme.com")
	if !ok {
		t.Fatalf("expected token lookup to succeed")
	}
	if token != "user_1@example.com" {
		t.Fatalf("token = %q, want %q", token, "user_1@example.com")
	}

	original, ok := vault.GetOriginal("user_1@example.com")
	if !ok {
		t.Fatalf("expected original lookup to succeed")
	}
	if original != "john@acme.com" {
		t.Fatalf("original = %q, want %q", original, "john@acme.com")
	}

	if size := vault.Size(); size != 1 {
		t.Fatalf("size = %d, want %d", size, 1)
	}
}

func TestVaultManagerGetOrCreateReturnsExisting(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	sameVault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault retrieval to succeed, got error: %v", err)
	}
	if vault != sameVault {
		t.Fatalf("expected GetOrCreate to return existing vault")
	}
}

func TestVaultManagerRefreshesTTLOnAccess(t *testing.T) {
	manager := NewVaultManager(80 * time.Millisecond)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	vault.mu.RLock()
	initialExpiry := vault.expiresAt
	vault.mu.RUnlock()

	time.Sleep(10 * time.Millisecond)

	_, err = manager.Get(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault retrieval to succeed, got error: %v", err)
	}

	vault.mu.RLock()
	refreshedExpiry := vault.expiresAt
	vault.mu.RUnlock()

	if !refreshedExpiry.After(initialExpiry) {
		t.Fatalf("expected expiry to refresh on access")
	}
}

func TestVaultManagerExpiresVaults(t *testing.T) {
	manager := NewVaultManager(20 * time.Millisecond)
	_, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	_, err = manager.Get(sampleSessionID)
	if err != ErrSessionExpired && err != ErrVaultNotFound {
		t.Fatalf("expected ErrSessionExpired or ErrVaultNotFound, got %v", err)
	}
}

func TestVaultManagerGetOrCreateEvictsExpired(t *testing.T) {
	manager := NewVaultManager(15 * time.Millisecond)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	time.Sleep(25 * time.Millisecond)

	newVault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}
	if vault == newVault {
		t.Fatalf("expected expired vault to be evicted")
	}
}

func TestVaultManagerCleanupRemovesExpired(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	_, err = manager.GetOrCreate(sampleSessionID2)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	vault.mu.Lock()
	vault.expiresAt = time.Now().Add(-time.Second)
	vault.mu.Unlock()

	manager.Cleanup()

	_, err = manager.Get(sampleSessionID)
	if err != ErrVaultNotFound {
		t.Fatalf("expected ErrVaultNotFound, got %v", err)
	}

	_, err = manager.Get(sampleSessionID2)
	if err != nil {
		t.Fatalf("expected vault to remain, got error: %v", err)
	}
}

func TestVaultStoreSubTokens(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	// Store a primary token first
	if err := vault.Store("eko@gmail.com", "usa@examp.eac"); err != nil {
		t.Fatalf("expected store to succeed, got error: %v", err)
	}

	// Store sub-tokens
	vault.StoreSubTokens(map[string]string{
		"examp.eac": "gmail.com",
		"eac":       "com",
		"usa":       "eko",
		"examp":     "gmail",
	})

	// Verify sub-tokens are retrievable via ReverseTokens
	reverse := vault.ReverseTokens()
	if reverse["eac"] != "com" {
		t.Fatalf("sub-token reverse[%q] = %q, want %q", "eac", reverse["eac"], "com")
	}
	if reverse["examp"] != "gmail" {
		t.Fatalf("sub-token reverse[%q] = %q, want %q", "examp", reverse["examp"], "gmail")
	}

	// Skip entries where fragment == original
	vault.StoreSubTokens(map[string]string{
		"same": "same",
	})
	reverse = vault.ReverseTokens()
	if _, exists := reverse["same"]; exists {
		t.Fatalf("expected identical fragment to be skipped")
	}

	// Conflict: store a different mapping for an existing sub-token
	vault.StoreSubTokens(map[string]string{
		"eac": "org", // conflicts with existing "eac" -> "com"
	})
	reverse = vault.ReverseTokens()
	if _, exists := reverse["eac"]; exists {
		t.Fatalf("expected conflicting sub-token to be deleted")
	}

	// Skip entries that collide with primary reverse tokens
	vault.StoreSubTokens(map[string]string{
		"usa@examp.eac": "should-be-skipped",
	})
	reverse = vault.ReverseTokens()
	if reverse["usa@examp.eac"] != "eko@gmail.com" {
		t.Fatalf("primary token should take precedence, got %q", reverse["usa@examp.eac"])
	}
}

func TestVaultReverseTokensIncludesSubTokens(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	// Store primary token
	if err := vault.Store("eko@gmail.com", "usa@examp.eac"); err != nil {
		t.Fatalf("expected store to succeed, got error: %v", err)
	}

	// Store sub-tokens
	vault.StoreSubTokens(map[string]string{
		"eac": "com",
		"usa": "eko",
	})

	reverse := vault.ReverseTokens()

	// Should contain primary token
	if reverse["usa@examp.eac"] != "eko@gmail.com" {
		t.Fatalf("expected primary token in reverse map")
	}

	// Should contain sub-tokens
	if reverse["eac"] != "com" {
		t.Fatalf("expected sub-token 'eac' in reverse map")
	}
	if reverse["usa"] != "eko" {
		t.Fatalf("expected sub-token 'usa' in reverse map")
	}

	// Primary takes precedence: store a sub-token that collides with a primary key
	if err := vault.Store("real-original", "usa"); err != nil {
		t.Fatalf("expected store to succeed, got error: %v", err)
	}
	reverse = vault.ReverseTokens()
	if reverse["usa"] != "real-original" {
		t.Fatalf("primary token should take precedence over sub-token, got %q", reverse["usa"])
	}
}

func TestVaultConcurrentAccess(t *testing.T) {
	manager := NewVaultManager(time.Minute)
	vault, err := manager.GetOrCreate(sampleSessionID)
	if err != nil {
		t.Fatalf("expected vault creation to succeed, got error: %v", err)
	}

	const entries = 50
	errCh := make(chan string, entries)
	var wg sync.WaitGroup
	wg.Add(entries)

	for i := 0; i < entries; i++ {
		idx := i
		go func() {
			defer wg.Done()
			original := fmt.Sprintf("value-%d", idx)
			token := fmt.Sprintf("token-%d", idx)
			if err := vault.Store(original, token); err != nil {
				errCh <- fmt.Sprintf("store failed: %v", err)
				return
			}

			if got, ok := vault.GetToken(original); !ok || got != token {
				errCh <- fmt.Sprintf("token lookup = %q, ok=%t", got, ok)
			}
			if got, ok := vault.GetOriginal(token); !ok || got != original {
				errCh <- fmt.Sprintf("original lookup = %q, ok=%t", got, ok)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Error(msg)
	}

	if size := vault.Size(); size != entries {
		t.Fatalf("size = %d, want %d", size, entries)
	}
}
