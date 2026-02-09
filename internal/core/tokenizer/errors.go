package tokenizer

import "errors"

var (
	ErrSessionExpired   = errors.New("session expired")
	ErrVaultNotFound    = errors.New("vault not found")
	ErrVaultFull        = errors.New("vault full")
	ErrInvalidSessionID = errors.New("invalid session id")
)
