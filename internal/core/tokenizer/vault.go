package tokenizer

import (
	"sync"
	"time"
)

type TokenMap struct {
	forward  map[string]string
	reverse  map[string]string
	counters map[string]uint64
}

type Vault struct {
	sessionID string
	expiresAt time.Time
	mu        sync.RWMutex
	tokens    TokenMap
	inflight  map[string]chan struct{}
	maxTokens int
}

type VaultManager struct {
	mu                sync.RWMutex
	vaults            map[string]*Vault
	ttl               time.Duration
	maxVaults         int
	maxTokensPerVault int
}

const (
	defaultMaxVaults         = 10000
	defaultMaxTokensPerVault = 100000
)

type VaultManagerOption func(*VaultManager)

func WithMaxVaults(maxVaults int) VaultManagerOption {
	return func(vm *VaultManager) {
		if maxVaults > 0 {
			vm.maxVaults = maxVaults
		}
	}
}

func WithMaxTokensPerVault(maxTokens int) VaultManagerOption {
	return func(vm *VaultManager) {
		if maxTokens > 0 {
			vm.maxTokensPerVault = maxTokens
		}
	}
}

func NewVaultManager(ttl time.Duration, opts ...VaultManagerOption) *VaultManager {
	vm := &VaultManager{
		vaults:            make(map[string]*Vault),
		ttl:               ttl,
		maxVaults:         defaultMaxVaults,
		maxTokensPerVault: defaultMaxTokensPerVault,
	}
	for _, opt := range opts {
		opt(vm)
	}
	return vm
}

func (vm *VaultManager) GetOrCreate(sessionID string) (*Vault, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

	now := time.Now()

	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vault, exists := vm.vaults[sessionID]; exists {
		if vault.isExpired(now) {
			delete(vm.vaults, sessionID)
		} else {
			vault.refresh(now, vm.ttl)
			return vault, nil
		}
	}

	if vm.maxVaults > 0 && len(vm.vaults) >= vm.maxVaults {
		vm.cleanupExpiredLocked(now)
		if len(vm.vaults) >= vm.maxVaults {
			return nil, ErrVaultFull
		}
	}

	vault := newVault(sessionID, now, vm.ttl, vm.maxTokensPerVault)
	vm.vaults[sessionID] = vault
	return vault, nil
}

func (vm *VaultManager) Get(sessionID string) (*Vault, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

	now := time.Now()

	vm.mu.Lock()
	defer vm.mu.Unlock()

	vault, exists := vm.vaults[sessionID]
	if !exists {
		return nil, ErrVaultNotFound
	}

	if vault.isExpired(now) {
		delete(vm.vaults, sessionID)
		return nil, ErrSessionExpired
	}

	vault.refresh(now, vm.ttl)
	return vault, nil
}

func (vm *VaultManager) Delete(sessionID string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	delete(vm.vaults, sessionID)
}

func (vm *VaultManager) Cleanup() {
	now := time.Now()

	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.cleanupExpiredLocked(now)
}

func (vm *VaultManager) cleanupExpiredLocked(now time.Time) {
	for sessionID, vault := range vm.vaults {
		if vault.isExpired(now) {
			delete(vm.vaults, sessionID)
		}
	}
}

func (v *Vault) Store(original, token string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, exists := v.tokens.forward[original]; !exists {
		if v.maxTokens > 0 && len(v.tokens.forward) >= v.maxTokens {
			return ErrVaultFull
		}
	}

	v.tokens.forward[original] = token
	v.tokens.reverse[token] = original
	return nil
}

func (v *Vault) GetToken(original string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	token, ok := v.tokens.forward[original]
	return token, ok
}

func (v *Vault) GetOriginal(token string) (string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	original, ok := v.tokens.reverse[token]
	return original, ok
}

func (v *Vault) ReverseTokens() map[string]string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(v.tokens.reverse) == 0 {
		return nil
	}

	reverse := make(map[string]string, len(v.tokens.reverse))
	for token, original := range v.tokens.reverse {
		reverse[token] = original
	}
	return reverse
}

func (v *Vault) Size() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return len(v.tokens.forward)
}

func newVault(sessionID string, now time.Time, ttl time.Duration, maxTokens int) *Vault {
	return &Vault{
		sessionID: sessionID,
		expiresAt: now.Add(ttl),
		tokens: TokenMap{
			forward:  make(map[string]string),
			reverse:  make(map[string]string),
			counters: make(map[string]uint64),
		},
		inflight:  make(map[string]chan struct{}),
		maxTokens: maxTokens,
	}
}

func (v *Vault) refresh(now time.Time, ttl time.Duration) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.expiresAt = now.Add(ttl)
}

func (v *Vault) isExpired(now time.Time) bool {
	v.mu.RLock()
	expiresAt := v.expiresAt
	v.mu.RUnlock()

	return !now.Before(expiresAt)
}
