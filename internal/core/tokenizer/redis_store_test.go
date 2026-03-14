package tokenizer

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisStoreRoundTrip(t *testing.T) {
	mini := miniredis.RunT(t)

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		KeyPrefix:     "test:",
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   testKeyProvider(t),
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	handle, err := store.BeginSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	vault := handle.Vault()
	if err := vault.Store("john@acme.com", "masked@example.com"); err != nil {
		t.Fatalf("store token: %v", err)
	}
	vault.StoreSubTokens(map[string]string{
		"example.com": "acme.com",
	})
	if err := handle.Save(context.Background()); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.GetSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got, ok := loaded.GetOriginal("masked@example.com"); !ok || got != "john@acme.com" {
		t.Fatalf("reverse lookup = %q, %v", got, ok)
	}
	reverse := loaded.ReverseTokens()
	if reverse["example.com"] != "acme.com" {
		t.Fatalf("expected subtoken mapping, got %#v", reverse)
	}
}

func TestRedisStoreHealthCheck(t *testing.T) {
	mini := miniredis.RunT(t)

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   testKeyProvider(t),
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	if err := store.HealthCheck(context.Background()); err != nil {
		t.Fatalf("health check: %v", err)
	}
}

func TestRedisStoreCorruptedPayloadDeletesKey(t *testing.T) {
	mini := miniredis.RunT(t)

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		KeyPrefix:     "test:",
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   testKeyProvider(t),
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	metaKey := "test:meta:" + resolverSessionID
	payloadKey := "test:payload:" + resolverSessionID
	mini.Set(metaKey, `{"schema_version":1,"payload_version":1,"key_id":"test-key","created_at_unix":1,"updated_at_unix":1}`)
	mini.Set(payloadKey, "{not-json")

	_, err = store.GetSession(context.Background(), resolverSessionID)
	if !errors.Is(err, ErrSessionStoreCorrupted) {
		t.Fatalf("expected corrupted state error, got %v", err)
	}
	if mini.Exists(metaKey) || mini.Exists(payloadKey) {
		t.Fatalf("expected corrupted session keys to be deleted")
	}
}

func TestRedisStoreUsesRequestContext(t *testing.T) {
	mini := miniredis.RunT(t)

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   testKeyProvider(t),
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = store.BeginSession(ctx, resolverSessionID)
	if !errors.Is(err, ErrSessionStoreUnavailable) {
		t.Fatalf("expected unavailable error for canceled context, got %v", err)
	}
}

func TestRedisStoreHasSessionDeletesMissingPayloadMetadata(t *testing.T) {
	mini := miniredis.RunT(t)

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		KeyPrefix:     "test:",
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   testKeyProvider(t),
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	metaKey := "test:meta:" + resolverSessionID
	mini.Set(metaKey, `{"schema_version":1,"payload_version":1,"key_id":"test-key","created_at_unix":1,"updated_at_unix":1}`)

	exists, err := store.HasSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("has session: %v", err)
	}
	if exists {
		t.Fatal("expected missing payload session to be treated as absent")
	}
	if mini.Exists(metaKey) {
		t.Fatalf("expected stale metadata key %q to be deleted", metaKey)
	}
}

func TestRedisStoreRoundTripWithVaultTransitProvider(t *testing.T) {
	mini := miniredis.RunT(t)

	masterKey := make([]byte, aeadKeySize)
	copy(masterKey, []byte("0123456789abcdef0123456789abcdef"))
	vaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/transit/encrypt/eko-session":
			var req vaultTransitEncryptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode encrypt request: %v", err)
			}
			plaintext, err := base64.StdEncoding.DecodeString(req.Plaintext)
			if err != nil {
				t.Fatalf("decode plaintext: %v", err)
			}
			ciphertext, nonce, err := encryptWithKey(masterKey, plaintext)
			if err != nil {
				t.Fatalf("encrypt data key: %v", err)
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
				t.Fatalf("decrypt wrapped key: %v", err)
			}
			resp := vaultTransitDecryptResponse{}
			resp.Data.Plaintext = base64.StdEncoding.EncodeToString(plaintext)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Fatalf("unexpected vault transit path %s", r.URL.Path)
		}
	}))
	defer vaultServer.Close()

	keyProvider, err := NewVaultTransitKeyProvider(VaultTransitKeyProviderConfig{
		Address:     vaultServer.URL,
		Token:       "token",
		Mount:       "transit",
		KeyName:     "eko-session",
		ActiveKeyID: "ignored-configured-id",
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new vault transit provider: %v", err)
	}

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		KeyPrefix:     "test:",
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   keyProvider,
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	handle, err := store.BeginSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := handle.Vault().Store("john@acme.com", "masked@example.com"); err != nil {
		t.Fatalf("store token: %v", err)
	}
	if err := handle.Save(context.Background()); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.GetSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got, ok := loaded.GetOriginal("masked@example.com"); !ok || got != "john@acme.com" {
		t.Fatalf("reverse lookup = %q, %v", got, ok)
	}
}

func TestRedisStoreRoundTripWithVaultTransitProviderAfterKeyChange(t *testing.T) {
	mini := miniredis.RunT(t)

	masterKey := make([]byte, aeadKeySize)
	copy(masterKey, []byte("0123456789abcdef0123456789abcdef"))
	vaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/transit/encrypt/legacy-session":
			var req vaultTransitEncryptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode encrypt request: %v", err)
			}
			plaintext, err := base64.StdEncoding.DecodeString(req.Plaintext)
			if err != nil {
				t.Fatalf("decode plaintext: %v", err)
			}
			ciphertext, nonce, err := encryptWithKey(masterKey, plaintext)
			if err != nil {
				t.Fatalf("encrypt data key: %v", err)
			}
			resp := vaultTransitEncryptResponse{}
			resp.Data.Ciphertext = "vault:v1:" + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(ciphertext)
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/transit/decrypt/legacy-session":
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
				t.Fatalf("decrypt wrapped key: %v", err)
			}
			resp := vaultTransitDecryptResponse{}
			resp.Data.Plaintext = base64.StdEncoding.EncodeToString(plaintext)
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/transit/decrypt/new-session":
			t.Fatalf("historical payload should be decrypted with stored key id, got %s", r.URL.Path)
		default:
			t.Fatalf("unexpected vault transit path %s", r.URL.Path)
		}
	}))
	defer vaultServer.Close()

	writerProvider, err := NewVaultTransitKeyProvider(VaultTransitKeyProviderConfig{
		Address: vaultServer.URL,
		Token:   "token",
		Mount:   "transit",
		KeyName: "legacy-session",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new writer provider: %v", err)
	}

	store, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		KeyPrefix:     "test:",
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   writerProvider,
	})
	if err != nil {
		t.Fatalf("new redis store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	handle, err := store.BeginSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := handle.Vault().Store("john@acme.com", "masked@example.com"); err != nil {
		t.Fatalf("store token: %v", err)
	}
	if err := handle.Save(context.Background()); err != nil {
		t.Fatalf("save: %v", err)
	}

	readerProvider, err := NewVaultTransitKeyProvider(VaultTransitKeyProviderConfig{
		Address: vaultServer.URL,
		Token:   "token",
		Mount:   "transit",
		KeyName: "new-session",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new reader provider: %v", err)
	}

	rotatedStore, err := NewRedisStore(RedisStoreConfig{
		Addr:          mini.Addr(),
		KeyPrefix:     "test:",
		MetaSuffix:    "meta:",
		PayloadSuffix: "payload:",
		TTL:           time.Minute,
		MaxTokens:     10,
		HealthTimeout: time.Second,
		KeyProvider:   readerProvider,
	})
	if err != nil {
		t.Fatalf("new rotated redis store: %v", err)
	}
	defer func() {
		_ = rotatedStore.Close()
	}()

	loaded, err := rotatedStore.GetSession(context.Background(), resolverSessionID)
	if err != nil {
		t.Fatalf("get after key change: %v", err)
	}
	if got, ok := loaded.GetOriginal("masked@example.com"); !ok || got != "john@acme.com" {
		t.Fatalf("reverse lookup = %q, %v", got, ok)
	}
}

func TestStaticKeyProviderFallbackDecryptsRotatedKey(t *testing.T) {
	oldKey := make([]byte, aeadKeySize)
	newKey := make([]byte, aeadKeySize)
	if _, err := rand.Read(oldKey); err != nil {
		t.Fatalf("generate old key: %v", err)
	}
	if _, err := rand.Read(newKey); err != nil {
		t.Fatalf("generate new key: %v", err)
	}

	oldProvider, err := NewStaticKeyProvider("old", oldKey)
	if err != nil {
		t.Fatalf("old provider: %v", err)
	}
	wrapped, nonce, keyID, err := oldProvider.WrapDataKey(context.Background(), []byte("secret-data-key-32-byte-secret-data"))
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}

	rotatedProvider, err := NewStaticKeyProviderWithFallback("new", newKey, map[string][]byte{"old": oldKey})
	if err != nil {
		t.Fatalf("rotated provider: %v", err)
	}
	unwrapped, err := rotatedProvider.UnwrapDataKey(context.Background(), keyID, wrapped, nonce)
	if err != nil {
		t.Fatalf("unwrap with fallback: %v", err)
	}
	if string(unwrapped) != "secret-data-key-32-byte-secret-data" {
		t.Fatalf("unexpected unwrapped key %q", string(unwrapped))
	}
}

func testKeyProvider(t *testing.T) KeyProvider {
	t.Helper()

	raw := make([]byte, aeadKeySize)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	provider, err := NewStaticKeyProviderFromBase64("test-key", encoded)
	if err != nil {
		t.Fatalf("new static key provider: %v", err)
	}
	return provider
}
