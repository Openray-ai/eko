package tokenizer

import "errors"

var (
	ErrSessionExpired         = errors.New("session expired")
	ErrVaultNotFound          = errors.New("vault not found")
	ErrVaultFull              = errors.New("vault full")
	ErrInvalidSessionID       = errors.New("invalid session id")
	ErrTokenLengthExceeded    = errors.New("token length exceeded")
	ErrCredentialTokenization = errors.New("credential tokenization not allowed")
)
