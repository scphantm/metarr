// Package passwordhash implements one-way password hashing: a random salt
// per password, SHA-256 over salt+password, and a constant-time verify.
package passwordhash

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

const saltBytesLength = 16

// Hash generates a new random salt and returns the salt and the resulting
// hash of password, both hex-encoded.
func Hash(password string) (salt, hash string, err error) {
	saltBytes := make([]byte, saltBytesLength)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", fmt.Errorf("passwordhash: generating salt: %w", err)
	}
	salt = hex.EncodeToString(saltBytes)
	return salt, hashWithSalt(password, salt), nil
}

// Verify reports whether password matches the given salt/hash pair, using
// a constant-time comparison to avoid leaking timing information.
func Verify(password, salt, hash string) bool {
	computed := hashWithSalt(password, salt)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(hash)) == 1
}

func hashWithSalt(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + password))
	return hex.EncodeToString(sum[:])
}

// GenerateRandomPassword returns a random alphanumeric password of the
// given length, suitable for a freshly bootstrapped admin account.
func GenerateRandomPassword(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("passwordhash: generating random password: %w", err)
	}

	password := make([]byte, length)
	for i, b := range raw {
		password[i] = charset[int(b)%len(charset)]
	}
	return string(password), nil
}
