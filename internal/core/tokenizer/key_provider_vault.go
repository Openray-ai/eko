package tokenizer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultVaultTransitMount = "transit"
	defaultVaultTransitPath  = "keys/eko-session"
)

type VaultTransitKeyProviderConfig struct {
	Address       string
	Token         string
	Namespace     string
	Mount         string
	KeyName       string
	ActiveKeyID   string
	Timeout       time.Duration
	TLSSkipVerify bool
}

type VaultTransitKeyProvider struct {
	address     string
	namespace   string
	mount       string
	keyName     string
	activeKeyID string
	client      *http.Client
	token       string
}

type vaultTransitEncryptRequest struct {
	Plaintext string `json:"plaintext"`
}

type vaultTransitEncryptResponse struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
	} `json:"data"`
}

type vaultTransitDecryptRequest struct {
	Ciphertext string `json:"ciphertext"`
}

type vaultTransitDecryptResponse struct {
	Data struct {
		Plaintext string `json:"plaintext"`
	} `json:"data"`
}

type vaultTransitErrorResponse struct {
	Errors []string `json:"errors"`
}

func NewVaultTransitKeyProvider(cfg VaultTransitKeyProviderConfig) (*VaultTransitKeyProvider, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("%w: vault transit address is required", ErrSessionStoreUnavailable)
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("%w: vault transit token is required", ErrSessionStoreUnavailable)
	}
	if cfg.Mount == "" {
		cfg.Mount = defaultVaultTransitMount
	}
	if cfg.KeyName == "" {
		cfg.KeyName = defaultVaultTransitPath
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Second
	}
	stableKeyID := vaultTransitStableKeyID(strings.Trim(cfg.Mount, "/"), strings.Trim(cfg.KeyName, "/"))

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.TLSSkipVerify}

	return &VaultTransitKeyProvider{
		address:     strings.TrimRight(cfg.Address, "/"),
		namespace:   cfg.Namespace,
		mount:       strings.Trim(cfg.Mount, "/"),
		keyName:     strings.Trim(cfg.KeyName, "/"),
		activeKeyID: stableKeyID,
		token:       cfg.Token,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}, nil
}

func (p *VaultTransitKeyProvider) ActiveKeyID(context.Context) (string, error) {
	return p.activeKeyID, nil
}

func (p *VaultTransitKeyProvider) WrapDataKey(ctx context.Context, plaintextKey []byte) ([]byte, []byte, string, error) {
	reqBody := vaultTransitEncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString(plaintextKey),
	}
	var resp vaultTransitEncryptResponse
	if err := p.doTransitRequest(ctx, http.MethodPost, p.encryptPath(), reqBody, &resp); err != nil {
		return nil, nil, "", err
	}
	if resp.Data.Ciphertext == "" {
		return nil, nil, "", fmt.Errorf("%w: empty ciphertext from vault transit", ErrSessionStoreUnavailable)
	}
	return []byte(resp.Data.Ciphertext), nil, p.activeKeyID, nil
}

func (p *VaultTransitKeyProvider) UnwrapDataKey(ctx context.Context, keyID string, wrappedKey, nonce []byte) ([]byte, error) {
	if keyID == "" {
		return nil, fmt.Errorf("%w: missing vault transit key id", ErrSessionStoreCorrupted)
	}
	reqBody := vaultTransitDecryptRequest{
		Ciphertext: string(wrappedKey),
	}
	var resp vaultTransitDecryptResponse
	if err := p.doTransitRequest(ctx, http.MethodPost, p.decryptPath(), reqBody, &resp); err != nil {
		return nil, err
	}
	if resp.Data.Plaintext == "" {
		return nil, fmt.Errorf("%w: empty plaintext from vault transit", ErrSessionStoreCorrupted)
	}
	plaintext, err := base64.StdEncoding.DecodeString(resp.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64 plaintext from vault transit: %v", ErrSessionStoreCorrupted, err)
	}
	return plaintext, nil
}

func (p *VaultTransitKeyProvider) encryptPath() string {
	return fmt.Sprintf("/v1/%s/encrypt/%s", p.mount, p.keyName)
}

func (p *VaultTransitKeyProvider) decryptPath() string {
	return fmt.Sprintf("/v1/%s/decrypt/%s", p.mount, p.keyName)
}

func (p *VaultTransitKeyProvider) doTransitRequest(ctx context.Context, method, path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: failed to encode vault transit request: %v", ErrSessionStoreUnavailable, err)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.address+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: failed to create vault transit request: %v", ErrSessionStoreUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", p.token)
	if p.namespace != "" {
		req.Header.Set("X-Vault-Namespace", p.namespace)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: vault transit request failed: %v", ErrSessionStoreUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: failed to read vault transit response: %v", ErrSessionStoreUnavailable, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var vaultErr vaultTransitErrorResponse
		if json.Unmarshal(respBody, &vaultErr) == nil && len(vaultErr.Errors) > 0 {
			return fmt.Errorf("%w: vault transit error: %s", ErrSessionStoreUnavailable, strings.Join(vaultErr.Errors, "; "))
		}
		return fmt.Errorf("%w: vault transit returned status %d", ErrSessionStoreUnavailable, resp.StatusCode)
	}

	if err := json.Unmarshal(respBody, target); err != nil {
		return fmt.Errorf("%w: failed to decode vault transit response: %v", ErrSessionStoreCorrupted, err)
	}
	return nil
}

func vaultTransitStableKeyID(mount, keyName string) string {
	return fmt.Sprintf("vault-transit:%s/%s", mount, keyName)
}
