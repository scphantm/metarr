package jwt

import "github.com/golang-jwt/jwt/v5"

// Role is the access level for a JWT token.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleUser     Role = "user"
	RoleWebhook  Role = "webhook"
	RoleReadOnly Role = "read_only"
)

// Claims represents the standard JWT claims for Metarr authentication.
type Claims struct {
	Subject string `json:"sub"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}
