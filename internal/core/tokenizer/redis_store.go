package tokenizer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"eko/internal/helpers/logger"
	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisKeyPrefix     = "eko:"
	defaultRedisMetaSuffix    = "vault_meta:"
	defaultRedisPayloadSuffix = "vault_payload:"
	defaultRedisRetries       = 3
)

type RedisStoreConfig struct {
	Addr          string
	Username      string
	Password      string
	DB            int
	KeyPrefix     string
	MetaSuffix    string
	PayloadSuffix string
	PoolSize      int
	MinIdleConns  int
	DialTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	MaxRetries    int
	TTL           time.Duration
	MaxTokens     int
	HealthTimeout time.Duration
	KeyProvider   KeyProvider
}

type RedisStore struct {
	client        *redis.Client
	keyPrefix     string
	metaSuffix    string
	payloadSuffix string
	ttl           time.Duration
	maxTokens     int
	retries       int
	healthTTL     time.Duration
	keyProvider   KeyProvider
}

type redisSessionHandle struct {
	store           *RedisStore
	sessionID       string
	vault           *Vault
	expectedVersion uint64
	createdAt       int64
}

type redisVaultState struct {
	Version   uint64            `json:"version"`
	Forward   map[string]string `json:"forward"`
	Reverse   map[string]string `json:"reverse"`
	Counters  map[string]uint64 `json:"counters"`
	Subtokens map[string]string `json:"subtokens"`
}

type redisVaultMeta struct {
	SchemaVersion  int    `json:"schema_version"`
	PayloadVersion uint64 `json:"payload_version"`
	KeyID          string `json:"key_id"`
	CreatedAtUnix  int64  `json:"created_at_unix"`
	UpdatedAtUnix  int64  `json:"updated_at_unix"`
}

type redisVaultPayload struct {
	SchemaVersion   int    `json:"schema_version"`
	KeyID           string `json:"key_id"`
	WrappedDataKey  []byte `json:"wrapped_data_key"`
	WrappedKeyNonce []byte `json:"wrapped_key_nonce"`
	CiphertextNonce []byte `json:"ciphertext_nonce"`
	Ciphertext      []byte `json:"ciphertext"`
}

func NewRedisStore(cfg RedisStoreConfig) (*RedisStore, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("%w: redis address is required", ErrSessionStoreUnavailable)
	}
	if cfg.TTL <= 0 {
		return nil, fmt.Errorf("%w: ttl must be > 0", ErrSessionStoreUnavailable)
	}
	if cfg.KeyProvider == nil {
		return nil, fmt.Errorf("%w: redis key provider is required", ErrSessionStoreUnavailable)
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = defaultRedisKeyPrefix
	}
	if cfg.MetaSuffix == "" {
		cfg.MetaSuffix = defaultRedisMetaSuffix
	}
	if cfg.PayloadSuffix == "" {
		cfg.PayloadSuffix = defaultRedisPayloadSuffix
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultRedisRetries
	}
	if cfg.HealthTimeout <= 0 {
		cfg.HealthTimeout = 2 * time.Second
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	return &RedisStore{
		client:        client,
		keyPrefix:     cfg.KeyPrefix,
		metaSuffix:    cfg.MetaSuffix,
		payloadSuffix: cfg.PayloadSuffix,
		ttl:           cfg.TTL,
		maxTokens:     cfg.MaxTokens,
		retries:       cfg.MaxRetries,
		healthTTL:     cfg.HealthTimeout,
		keyProvider:   cfg.KeyProvider,
	}, nil
}

func (s *RedisStore) BeginSession(ctx context.Context, sessionID string) (SessionHandle, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

	meta, payload, err := s.loadSession(ctx, sessionID, true)
	if err != nil {
		if errors.Is(err, ErrVaultNotFound) {
			now := time.Now().Unix()
			return &redisSessionHandle{
				store:           s,
				sessionID:       sessionID,
				vault:           newVault(sessionID, time.Now(), s.ttl, s.maxTokens),
				expectedVersion: 0,
				createdAt:       now,
			}, nil
		}
		return nil, err
	}

	vaultState, err := s.decryptPayload(ctx, meta, payload)
	if err != nil {
		s.handleDecryptFailure(ctx, sessionID, err)
		return nil, err
	}

	return &redisSessionHandle{
		store:           s,
		sessionID:       sessionID,
		vault:           vaultState.toVault(sessionID, s.ttl, s.maxTokens),
		expectedVersion: meta.PayloadVersion,
		createdAt:       meta.CreatedAtUnix,
	}, nil
}

func (s *RedisStore) GetSession(ctx context.Context, sessionID string) (*Vault, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return nil, err
	}

	meta, payload, err := s.loadSession(ctx, sessionID, true)
	if err != nil {
		return nil, err
	}

	state, err := s.decryptPayload(ctx, meta, payload)
	if err != nil {
		s.handleDecryptFailure(ctx, sessionID, err)
		return nil, err
	}
	return state.toVault(sessionID, s.ttl, s.maxTokens), nil
}

// handleDecryptFailure decides whether to destroy the underlying Redis keys
// after a payload decode/decrypt failure. Schema/encoding corruption is
// unrecoverable and gets cleaned up; AEAD or unknown-key-id failures may
// indicate a key misconfiguration during rotation, so the ciphertext is
// preserved and the failure is logged at ERROR level for operator action.
func (s *RedisStore) handleDecryptFailure(ctx context.Context, sessionID string, err error) {
	if errors.Is(err, ErrSessionStoreCryptoFailure) {
		logger.Error("Session payload failed AEAD verification; preserving ciphertext", logger.Fields{
			"error":      err.Error(),
			"session_id": sessionID,
		})
		return
	}
	if errors.Is(err, ErrSessionStoreCorrupted) {
		_ = s.deleteKeys(ctx, sessionID)
	}
}

func (s *RedisStore) HasSession(ctx context.Context, sessionID string) (bool, error) {
	if err := ValidateSessionID(sessionID); err != nil {
		return false, err
	}

	result, err := touchSessionScript.Run(ctx, s.client, []string{s.metaKey(sessionID), s.payloadKey(sessionID)}, s.ttl.Milliseconds()).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, wrapStoreError(err)
	}
	return result != nil, nil
}

func (s *RedisStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := ValidateSessionID(sessionID); err != nil {
		return err
	}
	return s.deleteKeys(ctx, sessionID)
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.healthTTL)
	defer cancel()
	if err := s.client.Ping(ctx).Err(); err != nil {
		return wrapStoreError(err)
	}
	return nil
}

func (s *RedisStore) ConflictRetries() int {
	if s.retries <= 0 {
		return defaultRedisRetries
	}
	return s.retries
}

func (h *redisSessionHandle) Vault() *Vault {
	return h.vault
}

func (h *redisSessionHandle) Save(ctx context.Context) error {
	nowUnix := time.Now().Unix()
	meta, payload, err := h.store.encryptState(ctx, h.vault, h.expectedVersion+1, h.createdAt, nowUnix)
	if err != nil {
		return err
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("%w: failed to encode redis metadata: %v", ErrSessionStoreCorrupted, err)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: failed to encode redis payload: %v", ErrSessionStoreCorrupted, err)
	}

	err = h.store.client.Watch(ctx, func(tx *redis.Tx) error {
		metaKey := h.store.metaKey(h.sessionID)
		payloadKey := h.store.payloadKey(h.sessionID)

		rawMeta, getErr := tx.Get(ctx, metaKey).Bytes()
		if getErr != nil && !errors.Is(getErr, redis.Nil) {
			return wrapStoreError(getErr)
		}

		currentVersion := uint64(0)
		if getErr == nil {
			currentMeta, decodeErr := decodeRedisVaultMeta(rawMeta)
			if decodeErr != nil {
				return decodeErr
			}
			currentVersion = currentMeta.PayloadVersion
		}

		if currentVersion != h.expectedVersion {
			return ErrSessionStoreConflict
		}

		pipe := tx.TxPipeline()
		pipe.Set(ctx, metaKey, metaBytes, h.store.ttl)
		pipe.Set(ctx, payloadKey, payloadBytes, h.store.ttl)
		_, execErr := pipe.Exec(ctx)
		return execErr
	}, h.store.metaKey(h.sessionID), h.store.payloadKey(h.sessionID))
	if err == nil {
		h.expectedVersion = meta.PayloadVersion
		h.createdAt = meta.CreatedAtUnix
		return nil
	}
	if errors.Is(err, redis.TxFailedErr) || errors.Is(err, ErrSessionStoreConflict) {
		return ErrSessionStoreConflict
	}
	return wrapStoreError(err)
}

func (s *RedisStore) loadSession(ctx context.Context, sessionID string, touchTTL bool) (*redisVaultMeta, *redisVaultPayload, error) {
	metaKey := s.metaKey(sessionID)
	payloadKey := s.payloadKey(sessionID)

	var result []interface{}
	if touchTTL {
		raw, err := loadSessionScript.Run(ctx, s.client, []string{metaKey, payloadKey}, s.ttl.Milliseconds()).Result()
		if err != nil {
			return nil, nil, wrapStoreError(err)
		}
		var ok bool
		result, ok = raw.([]interface{})
		if !ok || len(result) != 2 {
			return nil, nil, fmt.Errorf("%w: invalid redis session payload response", ErrSessionStoreCorrupted)
		}
	} else {
		pipe := s.client.Pipeline()
		metaCmd := pipe.Get(ctx, metaKey)
		payloadCmd := pipe.Get(ctx, payloadKey)
		_, err := pipe.Exec(ctx)
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, nil, wrapStoreError(err)
		}
		result = []interface{}{metaCmd.Val(), payloadCmd.Val()}
	}

	metaBytes, ok := redisResultBytes(result[0])
	if !ok {
		return nil, nil, ErrVaultNotFound
	}
	payloadBytes, payloadOk := redisResultBytes(result[1])
	if !payloadOk {
		_ = s.deleteKeys(ctx, sessionID)
		return nil, nil, fmt.Errorf("%w: payload missing for active session", ErrSessionStoreCorrupted)
	}

	meta, err := decodeRedisVaultMeta(metaBytes)
	if err != nil {
		_ = s.deleteKeys(ctx, sessionID)
		return nil, nil, err
	}
	payload, err := decodeRedisVaultPayload(payloadBytes)
	if err != nil {
		_ = s.deleteKeys(ctx, sessionID)
		return nil, nil, err
	}

	return meta, payload, nil
}

func (s *RedisStore) encryptState(ctx context.Context, vault *Vault, version uint64, createdAtUnix, updatedAtUnix int64) (*redisVaultMeta, *redisVaultPayload, error) {
	state := newRedisVaultState(vault, version)
	plaintext, err := json.Marshal(state)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to encode vault state: %v", ErrSessionStoreCorrupted, err)
	}

	dataKey, err := generateDataKey()
	if err != nil {
		return nil, nil, err
	}
	wrappedKey, wrappedNonce, keyID, err := s.keyProvider.WrapDataKey(ctx, dataKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: failed to wrap data key: %v", ErrSessionStoreUnavailable, err)
	}
	ciphertext, ciphertextNonce, err := encryptWithKey(dataKey, plaintext)
	if err != nil {
		return nil, nil, err
	}

	if createdAtUnix == 0 {
		createdAtUnix = updatedAtUnix
	}

	meta := &redisVaultMeta{
		SchemaVersion:  defaultSchemaV1,
		PayloadVersion: version,
		KeyID:          keyID,
		CreatedAtUnix:  createdAtUnix,
		UpdatedAtUnix:  updatedAtUnix,
	}
	payload := &redisVaultPayload{
		SchemaVersion:   defaultSchemaV1,
		KeyID:           keyID,
		WrappedDataKey:  wrappedKey,
		WrappedKeyNonce: wrappedNonce,
		CiphertextNonce: ciphertextNonce,
		Ciphertext:      ciphertext,
	}
	return meta, payload, nil
}

func (s *RedisStore) decryptPayload(ctx context.Context, meta *redisVaultMeta, payload *redisVaultPayload) (*redisVaultState, error) {
	if meta == nil || payload == nil {
		return nil, ErrSessionStoreCorrupted
	}
	if meta.SchemaVersion != defaultSchemaV1 || payload.SchemaVersion != defaultSchemaV1 {
		return nil, fmt.Errorf("%w: unsupported redis schema version", ErrSessionStoreCorrupted)
	}
	if meta.KeyID != payload.KeyID {
		return nil, fmt.Errorf("%w: metadata/payload key mismatch", ErrSessionStoreCorrupted)
	}

	dataKey, err := s.keyProvider.UnwrapDataKey(ctx, payload.KeyID, payload.WrappedDataKey, payload.WrappedKeyNonce)
	if err != nil {
		if errors.Is(err, ErrSessionStoreCorrupted) || errors.Is(err, ErrSessionStoreCryptoFailure) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: failed to unwrap data key: %v", ErrSessionStoreUnavailable, err)
	}
	plaintext, err := decryptWithKey(dataKey, payload.Ciphertext, payload.CiphertextNonce)
	if err != nil {
		return nil, err
	}

	var state redisVaultState
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return nil, fmt.Errorf("%w: failed to decode decrypted vault state: %v", ErrSessionStoreCorrupted, err)
	}
	if state.Version != meta.PayloadVersion {
		return nil, fmt.Errorf("%w: metadata/payload version mismatch", ErrSessionStoreCorrupted)
	}
	if state.Forward == nil {
		state.Forward = make(map[string]string)
	}
	if state.Reverse == nil {
		state.Reverse = make(map[string]string)
	}
	if state.Counters == nil {
		state.Counters = make(map[string]uint64)
	}
	if state.Subtokens == nil {
		state.Subtokens = make(map[string]string)
	}
	return &state, nil
}

func (s *RedisStore) metaKey(sessionID string) string {
	return s.keyPrefix + s.metaSuffix + sessionID
}

func (s *RedisStore) payloadKey(sessionID string) string {
	return s.keyPrefix + s.payloadSuffix + sessionID
}

func (s *RedisStore) deleteKeys(ctx context.Context, sessionID string) error {
	if err := s.client.Del(ctx, s.metaKey(sessionID), s.payloadKey(sessionID)).Err(); err != nil {
		return wrapStoreError(err)
	}
	return nil
}

func newRedisVaultState(vault *Vault, version uint64) *redisVaultState {
	vault.mu.RLock()
	defer vault.mu.RUnlock()

	return &redisVaultState{
		Version:   version,
		Forward:   cloneStringMap(vault.tokens.forward),
		Reverse:   cloneStringMap(vault.tokens.reverse),
		Counters:  cloneUint64Map(vault.tokens.counters),
		Subtokens: cloneStringMap(vault.tokens.subtokens),
	}
}

func decodeRedisVaultMeta(raw []byte) (*redisVaultMeta, error) {
	var meta redisVaultMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("%w: failed to decode redis metadata: %v", ErrSessionStoreCorrupted, err)
	}
	if meta.KeyID == "" || meta.SchemaVersion == 0 {
		return nil, fmt.Errorf("%w: invalid redis metadata", ErrSessionStoreCorrupted)
	}
	return &meta, nil
}

func decodeRedisVaultPayload(raw []byte) (*redisVaultPayload, error) {
	var payload redisVaultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("%w: failed to decode redis payload: %v", ErrSessionStoreCorrupted, err)
	}
	if payload.KeyID == "" || payload.SchemaVersion == 0 || len(payload.WrappedDataKey) == 0 || len(payload.CiphertextNonce) == 0 || len(payload.Ciphertext) == 0 {
		return nil, fmt.Errorf("%w: invalid redis payload", ErrSessionStoreCorrupted)
	}
	return &payload, nil
}

func (s *redisVaultState) toVault(sessionID string, ttl time.Duration, maxTokens int) *Vault {
	return &Vault{
		sessionID: sessionID,
		expiresAt: time.Now().Add(ttl),
		tokens: TokenMap{
			forward:   cloneStringMap(s.Forward),
			reverse:   cloneStringMap(s.Reverse),
			counters:  cloneUint64Map(s.Counters),
			subtokens: cloneStringMap(s.Subtokens),
		},
		inflight:  make(map[string]chan struct{}),
		maxTokens: maxTokens,
	}
}

func redisResultBytes(value interface{}) ([]byte, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case string:
		if v == "" {
			return nil, false
		}
		return []byte(v), true
	case []byte:
		if len(v) == 0 {
			return nil, false
		}
		return v, true
	case bool:
		return nil, false
	default:
		return nil, false
	}
}

func wrapStoreError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrSessionStoreConflict),
		errors.Is(err, ErrSessionStoreCorrupted),
		errors.Is(err, ErrSessionStoreCryptoFailure),
		errors.Is(err, ErrVaultNotFound),
		errors.Is(err, ErrInvalidSessionID):
		return err
	default:
		return fmt.Errorf("%w: %v", ErrSessionStoreUnavailable, err)
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return make(map[string]string)
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneUint64Map(src map[string]uint64) map[string]uint64 {
	if len(src) == 0 {
		return make(map[string]uint64)
	}
	dst := make(map[string]uint64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
