package tokenizer

import "strings"

type Resolver struct {
	vaultManager *VaultManager
}

func NewResolver(vm *VaultManager) *Resolver {
	return &Resolver{vaultManager: vm}
}

func (r *Resolver) ResolveResponse(body []byte, sessionID string) ([]byte, error) {
	vault, err := r.vaultManager.Get(sessionID)
	if err != nil {
		return body, err
	}

	if len(body) == 0 {
		return body, nil
	}

	vault.mu.RLock()
	if len(vault.tokens.reverse) == 0 {
		vault.mu.RUnlock()
		return body, nil
	}

	reverse := make(map[string]string, len(vault.tokens.reverse))
	for token, original := range vault.tokens.reverse {
		reverse[token] = original
	}
	vault.mu.RUnlock()

	result := string(body)
	for token, original := range reverse {
		result = strings.ReplaceAll(result, token, original)
	}

	return []byte(result), nil
}
