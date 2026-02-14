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

	reverse := vault.ReverseTokens()
	if len(reverse) == 0 {
		return body, nil
	}

	pairs := make([]string, 0, len(reverse)*2)
	for token, original := range reverse {
		pairs = append(pairs, token, original)
	}

	result := strings.NewReplacer(pairs...).Replace(string(body))
	return []byte(result), nil
}
