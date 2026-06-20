package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateSessionToken generates a cryptographically secure 32-byte session token encoded in hex.
func GenerateSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
