// Package authsecret is the one place the JWT HMAC signing secret is read,
// base64-decoded and cached. Callers sign and verify tokens through Sign and
// Verify; the raw secret bytes never cross this package's boundary, so no
// call site can log or leak the key.
//
// The secret lives in Auth.HmacSecret on the live application-config
// singleton (internal/shared/appconfig). This package reads it there the
// same way its former call sites did — no dependency injection — and
// re-decodes only when the encoded value actually changes, matching the
// cache the Connect auth interceptor used to keep for itself. Token shape,
// algorithm and expiry validation stay in internal/server/jwt; this package
// only supplies the key.
package authsecret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"Metarr/internal/server/jwt"
	"Metarr/internal/shared/appconfig"
)

// secretBytes is the length of a raw HMAC-SHA256 signing secret. Bootstrap
// seeds the initial secret at this length and RotateHmacSecret replaces it
// with another of the same length.
const secretBytes = 32

// GenerateSecret returns a fresh, base64-encoded 32-byte HMAC signing secret
// read from crypto/rand — the exact format stored in Auth.HmacSecret. It is
// the one generator for that value: bootstrap seeds the first secret with it,
// and RotateHmacSecret replaces the live one with it.
func GenerateSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("authsecret: generating secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// ErrSecretNotConfigured is returned by Sign and Verify when the live config
// carries no HMAC secret at all — the same condition the inline
// `cfg.Auth == nil || cfg.Auth.HmacSecret == ""` checks used to guard.
var ErrSecretNotConfigured = errors.New("authsecret: hmac secret not configured")

// ErrSecretMalformed is returned when Auth.HmacSecret is set but is not valid
// base64 — a corrupted stored secret, distinct from one that is simply
// absent.
var ErrSecretMalformed = errors.New("authsecret: hmac secret is not valid base64")

// cache memoises the base64 decode of the configured secret. Sign and Verify
// run on every login, token issuance and authenticated RPC, and the secret
// only changes when config is re-propagated in-process, so decoding once per
// distinct encoded value keeps a per-call base64 allocation off the hot
// path.
var cache struct {
	mu      sync.RWMutex
	encoded string
	decoded []byte
}

// secret returns the raw bytes of the live HMAC secret, reusing the last
// decode while the encoded value is unchanged.
func secret() ([]byte, error) {
	cfg := appconfig.Get()
	if cfg.GetAuth() == nil || cfg.GetAuth().GetHmacSecret() == "" {
		return nil, ErrSecretNotConfigured
	}
	encoded := cfg.GetAuth().GetHmacSecret()

	cache.mu.RLock()
	if cache.encoded == encoded && cache.decoded != nil {
		decoded := cache.decoded
		cache.mu.RUnlock()
		return decoded, nil
	}
	cache.mu.RUnlock()

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSecretMalformed, err)
	}

	cache.mu.Lock()
	cache.encoded = encoded
	cache.decoded = decoded
	cache.mu.Unlock()
	return decoded, nil
}

// Sign issues a signed JWT for subject with the given role and ttl (seconds),
// using the live HMAC secret. It mirrors jwt.SignJWT minus the secret
// parameter.
func Sign(subject, role string, ttl int32) (string, error) {
	key, err := secret()
	if err != nil {
		return "", err
	}
	return jwt.SignJWT(subject, role, ttl, key)
}

// Verify parses and verifies token against the live HMAC secret, returning
// its claims. It mirrors jwt.VerifyJWT minus the secret parameter.
func Verify(token string) (*jwt.Claims, error) {
	key, err := secret()
	if err != nil {
		return nil, err
	}
	return jwt.VerifyJWT(token, key)
}
