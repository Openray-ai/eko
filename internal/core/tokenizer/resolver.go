package tokenizer

import (
	"sort"
	"strings"
)

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

	tokens := make([]string, 0, len(reverse))
	for token := range reverse {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		return len(tokens[i]) > len(tokens[j])
	})

	result := string(body)
	for _, token := range tokens {
		result = strings.ReplaceAll(result, token, reverse[token])
	}

	return []byte(result), nil
}
