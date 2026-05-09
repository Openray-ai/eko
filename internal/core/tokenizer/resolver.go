package tokenizer

import (
	"context"
	"strings"
)

type Resolver struct {
	store SessionStore
}

func NewResolver(store SessionStore) *Resolver {
	return &Resolver{store: store}
}

func (r *Resolver) ResolveResponse(body []byte, sessionID string) ([]byte, error) {
	return r.ResolveResponseWithContext(context.Background(), body, sessionID)
}

func (r *Resolver) ResolveResponseWithContext(ctx context.Context, body []byte, sessionID string) ([]byte, error) {
	vault, err := r.store.GetSession(ctx, sessionID)
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
