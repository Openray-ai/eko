package tokenizer

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	aeadNonceSize   = 12
	aeadKeySize     = 32
	defaultSchemaV1 = 1
)

type KeyProvider interface {
	ActiveKeyID(ctx context.Context) (string, error)
	WrapDataKey(ctx context.Context, plaintextKey []byte) (wrappedKey []byte, nonce []byte, keyID string, err error)
	UnwrapDataKey(ctx context.Context, keyID string, wrappedKey, nonce []byte) ([]byte, error)
}

type StaticKeyProvider struct {
	activeKeyID string
	keys        map[string][]byte
}

func NewStaticKeyProvider(activeKeyID string, masterKey []byte) (*StaticKeyProvider, error) {
	return NewStaticKeyProviderWithFallback(activeKeyID, masterKey, nil)
}

func NewStaticKeyProviderWithFallback(activeKeyID string, masterKey []byte, fallback map[string][]byte) (*StaticKeyProvider, error) {
	if activeKeyID == "" {
		return nil, fmt.Errorf("%w: active key id is required", ErrSessionStoreUnavailable)
	}
	if len(masterKey) != aeadKeySize {
		return nil, fmt.Errorf("%w: master key must be 32 bytes", ErrSessionStoreUnavailable)
	}

	keys := make(map[string][]byte, len(fallback)+1)
	keys[activeKeyID] = cloneBytes(masterKey)
	for keyID, key := range fallback {
		if keyID == "" {
			return nil, fmt.Errorf("%w: fallback key id is required", ErrSessionStoreUnavailable)
		}
		if len(key) != aeadKeySize {
			return nil, fmt.Errorf("%w: fallback key %q must be 32 bytes", ErrSessionStoreUnavailable, keyID)
		}
		keys[keyID] = cloneBytes(key)
	}

	return &StaticKeyProvider{
		activeKeyID: activeKeyID,
		keys:        keys,
	}, nil
}

func NewStaticKeyProviderFromBase64(activeKeyID, encoded string) (*StaticKeyProvider, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 master key: %v", ErrSessionStoreUnavailable, err)
	}
	return NewStaticKeyProvider(activeKeyID, raw)
}

func NewStaticKeyProviderWithFallbackBase64(activeKeyID, encoded string, fallback map[string]string) (*StaticKeyProvider, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 master key: %v", ErrSessionStoreUnavailable, err)
	}
	decodedFallback := make(map[string][]byte, len(fallback))
	for keyID, value := range fallback {
		key, decodeErr := base64.StdEncoding.DecodeString(value)
		if decodeErr != nil {
			return nil, fmt.Errorf("%w: invalid fallback key %q: %v", ErrSessionStoreUnavailable, keyID, decodeErr)
		}
		decodedFallback[keyID] = key
	}
	return NewStaticKeyProviderWithFallback(activeKeyID, raw, decodedFallback)
}

func (p *StaticKeyProvider) ActiveKeyID(context.Context) (string, error) {
	return p.activeKeyID, nil
}

func (p *StaticKeyProvider) WrapDataKey(_ context.Context, plaintextKey []byte) ([]byte, []byte, string, error) {
	masterKey, ok := p.keys[p.activeKeyID]
	if !ok {
		return nil, nil, "", fmt.Errorf("%w: active key %q not configured", ErrSessionStoreUnavailable, p.activeKeyID)
	}
	wrapped, nonce, err := encryptWithKey(masterKey, plaintextKey)
	if err != nil {
		return nil, nil, "", err
	}
	return wrapped, nonce, p.activeKeyID, nil
}

func (p *StaticKeyProvider) UnwrapDataKey(_ context.Context, keyID string, wrappedKey, nonce []byte) ([]byte, error) {
	masterKey, ok := p.keys[keyID]
	if !ok {
		// Treated as a recoverable crypto failure (likely missing fallback during rotation)
		// rather than corruption, so the caller does not delete the underlying data.
		return nil, fmt.Errorf("%w: unknown key id %q", ErrSessionStoreCryptoFailure, keyID)
	}
	return decryptWithKey(masterKey, wrappedKey, nonce)
}

func generateDataKey() ([]byte, error) {
	key := make([]byte, aeadKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("%w: failed to generate data key: %v", ErrSessionStoreUnavailable, err)
	}
	return key, nil
}

func encryptWithKey(key, plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to initialize cipher: %v", ErrSessionStoreUnavailable, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to initialize AEAD: %v", ErrSessionStoreUnavailable, err)
	}

	nonce := make([]byte, aeadNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("%w: failed to generate nonce: %v", ErrSessionStoreUnavailable, err)
	}

	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func decryptWithKey(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to initialize cipher: %v", ErrSessionStoreCryptoFailure, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to initialize AEAD: %v", ErrSessionStoreCryptoFailure, err)
	}
	// AEAD verification failure may indicate tampering OR a key misconfiguration
	// (wrong active key, missing rotation fallback). Surface as crypto failure so
	// the caller leaves the ciphertext intact for operator inspection / recovery.
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decrypt payload: %v", ErrSessionStoreCryptoFailure, err)
	}
	return plaintext, nil
}

func cloneBytes(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
