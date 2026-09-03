package jwt

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// VerifyJWT parses and verifies the authenticity and validity of a JWT token.
// Returns the decoded Claims if the token is valid, or an error if verification fails.
// Possible errors include:
// - Malformed token (invalid format)
// - Invalid signature (token was tampered with or signed with different secret)
// - Expired token (exp claim in the past)
// - Invalid claims (required claims missing or invalid)
func VerifyJWT(tokenString string, secret []byte) (*Claims, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("token string must not be empty")
	}
	if len(secret) == 0 {
		return nil, fmt.Errorf("secret must not be empty")
	}

	claims := &Claims{}
	parsedToken, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !parsedToken.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	if claims.Subject == "" {
		return nil, fmt.Errorf("token missing subject claim")
	}
	if claims.Role == "" {
		return nil, fmt.Errorf("token missing role claim")
	}

	return claims, nil
}
