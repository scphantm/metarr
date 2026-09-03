package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SignJWT creates and signs a JWT token with the given parameters.
// subject is typically the username or principal identifier.
// role is the access level (admin, user, webhook, read_only).
// ttl is the token lifetime in seconds.
// secret is the HMAC-SHA256 signing key (base64-encoded or raw bytes).
// Returns the signed JWT string and any error.
func SignJWT(subject string, role string, ttl int32, secret []byte) (string, error) {
	if subject == "" {
		return "", fmt.Errorf("subject must not be empty")
	}
	if role == "" {
		return "", fmt.Errorf("role must not be empty")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("ttl must be positive")
	}
	if len(secret) == 0 {
		return "", fmt.Errorf("secret must not be empty")
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(ttl) * time.Second)

	claims := Claims{
		Subject: subject,
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}
