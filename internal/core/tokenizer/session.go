package tokenizer

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

const sessionPrefix = "eko_"

// GenerateSessionID returns a new session id in the format eko_<uuid>.
func GenerateSessionID() string {
	return sessionPrefix + newUUID()
}

// ValidateSessionID validates that the session id is non-empty, has the
// correct prefix, and contains a valid UUID after the prefix.
func ValidateSessionID(sessionID string) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}
	if !strings.HasPrefix(sessionID, sessionPrefix) {
		return ErrInvalidSessionID
	}
	uuidPart := strings.TrimPrefix(sessionID, sessionPrefix)
	if uuidPart == "" {
		return ErrInvalidSessionID
	}
	if !isValidUUID(uuidPart) {
		return ErrInvalidSessionID
	}
	return nil
}

func isValidUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i := 0; i < len(value); i++ {
		switch i {
		case 8, 13, 18, 23:
			if value[i] != '-' {
				return false
			}
		default:
			if !isHex(value[i]) {
				return false
			}
		}
	}
	return true
}

func isHex(value byte) bool {
	return (value >= '0' && value <= '9') ||
		(value >= 'a' && value <= 'f') ||
		(value >= 'A' && value <= 'F')
}

func newUUID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		fillUUIDFallback(bytes)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func fillUUIDFallback(bytes []byte) {
	seed := time.Now().UnixNano()
	for i := 0; i < len(bytes); i++ {
		seed = seed*1664525 + 1013904223
		bytes[i] = byte(seed >> 24)
	}
}
