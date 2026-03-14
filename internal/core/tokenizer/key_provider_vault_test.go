package tokenizer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVaultTransitKeyProviderRoundTrip(t *testing.T) {
	masterKey := make([]byte, aeadKeySize)
	copy(masterKey, []byte("0123456789abcdef0123456789abcdef"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/transit/encrypt/eko-session":
			var req vaultTransitEncryptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode encrypt request: %v", err)
			}
			plaintext, err := base64.StdEncoding.DecodeString(req.Plaintext)
			if err != nil {
				t.Fatalf("decode encrypt plaintext: %v", err)
			}
			ciphertext, nonce, err := encryptWithKey(masterKey, plaintext)
			if err != nil {
				t.Fatalf("encrypt with key: %v", err)
			}
			resp := vaultTransitEncryptResponse{}
			resp.Data.Ciphertext = "vault:v1:" + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(ciphertext)
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/transit/decrypt/eko-session":
			var req vaultTransitDecryptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode decrypt request: %v", err)
			}
			parts := strings.Split(req.Ciphertext, ":")
			if len(parts) != 4 {
				t.Fatalf("unexpected ciphertext format %q", req.Ciphertext)
			}
			nonce, err := base64.StdEncoding.DecodeString(parts[2])
			if err != nil {
				t.Fatalf("decode nonce: %v", err)
			}
			ciphertext, err := base64.StdEncoding.DecodeString(parts[3])
			if err != nil {
				t.Fatalf("decode ciphertext: %v", err)
			}
			plaintext, err := decryptWithKey(masterKey, ciphertext, nonce)
			if err != nil {
				t.Fatalf("decrypt with key: %v", err)
			}
			resp := vaultTransitDecryptResponse{}
			resp.Data.Plaintext = base64.StdEncoding.EncodeToString(plaintext)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewVaultTransitKeyProvider(VaultTransitKeyProviderConfig{
		Address:     server.URL,
		Token:       "token",
		Mount:       "transit",
		KeyName:     "eko-session",
		ActiveKeyID: "vault-primary",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new vault provider: %v", err)
	}

	wrapped, nonce, keyID, err := provider.WrapDataKey(context.Background(), []byte("plaintext-data-key-32-bytes!!!!"))
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if len(nonce) != 0 {
		t.Fatalf("expected no nonce from vault provider, got %d bytes", len(nonce))
	}
	if keyID != "vault-transit:transit/eko-session" {
		t.Fatalf("key id = %q, want vault-transit:transit/eko-session", keyID)
	}

	unwrapped, err := provider.UnwrapDataKey(context.Background(), keyID, wrapped, nil)
	if err != nil {
		t.Fatalf("unwrap: %v", err)
	}
	if string(unwrapped) != "plaintext-data-key-32-bytes!!!!" {
		t.Fatalf("unexpected unwrapped key %q", string(unwrapped))
	}
}
