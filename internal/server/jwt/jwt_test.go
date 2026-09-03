package jwt

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSignJWT_ValidToken(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	subject := "testuser"
	role := "admin"
	ttl := int32(3600)

	token, err := SignJWT(subject, role, ttl, secret)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	// Verify the token is well-formed (has 3 parts)
	parts := 0
	for _, ch := range token {
		if ch == '.' {
			parts++
		}
	}
	if parts != 2 {
		t.Errorf("token should have 2 dots (3 parts), got %d dots", parts)
	}
}

func TestSignJWT_InvalidInputs(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")

	tests := []struct {
		name    string
		subject string
		role    string
		ttl     int32
		secret  []byte
		wantErr bool
	}{
		{
			name:    "empty subject",
			subject: "",
			role:    "admin",
			ttl:     3600,
			secret:  secret,
			wantErr: true,
		},
		{
			name:    "empty role",
			subject: "testuser",
			role:    "",
			ttl:     3600,
			secret:  secret,
			wantErr: true,
		},
		{
			name:    "zero ttl",
			subject: "testuser",
			role:    "admin",
			ttl:     0,
			secret:  secret,
			wantErr: true,
		},
		{
			name:    "negative ttl",
			subject: "testuser",
			role:    "admin",
			ttl:     -1,
			secret:  secret,
			wantErr: true,
		},
		{
			name:    "empty secret",
			subject: "testuser",
			role:    "admin",
			ttl:     3600,
			secret:  []byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SignJWT(tt.subject, tt.role, tt.ttl, tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("SignJWT() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyJWT_ValidToken(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	subject := "testuser"
	role := "admin"
	ttl := int32(3600)

	token, err := SignJWT(subject, role, ttl, secret)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	claims, err := VerifyJWT(token, secret)
	if err != nil {
		t.Fatalf("VerifyJWT failed: %v", err)
	}

	if claims.Subject != subject {
		t.Errorf("Subject mismatch: got %q, want %q", claims.Subject, subject)
	}
	if claims.Role != role {
		t.Errorf("Role mismatch: got %q, want %q", claims.Role, role)
	}
	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt should not be nil")
	}
	if claims.IssuedAt == nil {
		t.Error("IssuedAt should not be nil")
	}
}

func TestVerifyJWT_RoundTrip(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	tests := []struct {
		name    string
		subject string
		role    string
		ttl     int32
	}{
		{name: "admin", subject: "admin-user", role: "admin", ttl: 86400},
		{name: "user", subject: "regular-user", role: "user", ttl: 86400},
		{name: "webhook", subject: "webhook-service", role: "webhook", ttl: 604800},
		{name: "read_only", subject: "read-only-user", role: "read_only", ttl: 3600},
		{name: "long ttl", subject: "integration", role: "user", ttl: 31536000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := SignJWT(tt.subject, tt.role, tt.ttl, secret)
			if err != nil {
				t.Fatalf("SignJWT failed: %v", err)
			}

			claims, err := VerifyJWT(token, secret)
			if err != nil {
				t.Fatalf("VerifyJWT failed: %v", err)
			}

			if claims.Subject != tt.subject {
				t.Errorf("Subject mismatch: got %q, want %q", claims.Subject, tt.subject)
			}
			if claims.Role != tt.role {
				t.Errorf("Role mismatch: got %q, want %q", claims.Role, tt.role)
			}
		})
	}
}

func TestVerifyJWT_TamperedToken(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	subject := "testuser"
	role := "admin"
	ttl := int32(3600)

	token, err := SignJWT(subject, role, ttl, secret)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	// Tamper with the token by changing a character in the last part (signature)
	tamperedToken := token[:len(token)-1] + "X"

	_, err = VerifyJWT(tamperedToken, secret)
	if err == nil {
		t.Error("VerifyJWT should fail with tampered token")
	}
}

func TestVerifyJWT_WrongSecret(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	wrongSecret := []byte("wrong-secret-key-32-bytes-long")
	subject := "testuser"
	role := "admin"
	ttl := int32(3600)

	token, err := SignJWT(subject, role, ttl, secret)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}

	_, err = VerifyJWT(token, wrongSecret)
	if err == nil {
		t.Error("VerifyJWT should fail with wrong secret")
	}
}

func TestVerifyJWT_ExpiredToken(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	subject := "testuser"
	role := "admin"

	// Manually create an expired token since SignJWT doesn't allow negative TTL
	now := time.Now()
	expiresAt := now.Add(-time.Hour)

	claims := Claims{
		Subject: subject,
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			NotBefore: jwt.NewNumericDate(now.Add(-2 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}

	_, err = VerifyJWT(tokenString, secret)
	if err == nil {
		t.Error("VerifyJWT should fail with expired token")
	}
}

func TestVerifyJWT_MalformedToken(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")

	tests := []struct {
		name   string
		token  string
		secret []byte
		err    bool
	}{
		{name: "empty token", token: "", secret: secret, err: true},
		{name: "empty secret", token: "some.token.here", secret: []byte{}, err: true},
		{name: "invalid format", token: "not.a.valid.jwt", secret: secret, err: true},
		{name: "single part", token: "notavalidjwt", secret: secret, err: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := VerifyJWT(tt.token, tt.secret)
			if (err != nil) != tt.err {
				t.Errorf("VerifyJWT() error = %v, want error: %v", err, tt.err)
			}
		})
	}
}

func TestVerifyJWT_MissingClaims(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")

	// Create a token without role claim
	now := time.Now()
	expiresAt := now.Add(time.Hour)

	customClaims := jwt.MapClaims{
		"sub": "testuser",
		"exp": expiresAt.Unix(),
		"iat": now.Unix(),
		"nbf": now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, customClaims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = VerifyJWT(tokenString, secret)
	if err == nil {
		t.Error("VerifyJWT should fail when role claim is missing")
	}
}

func TestVerifyJWT_WrongSigningMethod(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	subject := "testuser"
	role := "admin"

	now := time.Now()
	expiresAt := now.Add(time.Hour)

	claims := jwt.MapClaims{
		"sub":  subject,
		"role": role,
		"exp":  expiresAt.Unix(),
		"iat":  now.Unix(),
		"nbf":  now.Unix(),
	}

	// Sign with HS512 instead of HS256
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = VerifyJWT(tokenString, secret)
	if err == nil {
		t.Error("VerifyJWT should fail with wrong signing method")
	}
}

func TestExpireAtTimestamp(t *testing.T) {
	secret := []byte("test-secret-key-32-bytes-long!")
	subject := "testuser"
	role := "admin"
	ttl := int32(3600)

	beforeSign := time.Now()
	token, err := SignJWT(subject, role, ttl, secret)
	if err != nil {
		t.Fatalf("SignJWT failed: %v", err)
	}
	afterSign := time.Now()

	claims, err := VerifyJWT(token, secret)
	if err != nil {
		t.Fatalf("VerifyJWT failed: %v", err)
	}

	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt should not be nil")
	}

	expiresTime := claims.ExpiresAt.Time
	expectedMin := beforeSign.Add(time.Duration(ttl) * time.Second)
	expectedMax := afterSign.Add(time.Duration(ttl) * time.Second)

	// Allow 2 second tolerance for test execution time
	if expiresTime.Before(expectedMin.Add(-time.Second)) || expiresTime.After(expectedMax.Add(time.Second)) {
		t.Errorf("ExpiresAt not within expected range: got %v, want between %v and %v",
			expiresTime, expectedMin, expectedMax)
	}
}

func TestBase64EncodedSecret(t *testing.T) {
	rawSecret := []byte("test-secret-key-32-bytes-long!")
	base64Secret := []byte(base64.StdEncoding.EncodeToString(rawSecret))
	subject := "testuser"
	role := "admin"
	ttl := int32(3600)

	// Sign with base64-encoded secret (treated as raw bytes)
	token, err := SignJWT(subject, role, ttl, base64Secret)
	if err != nil {
		t.Fatalf("SignJWT with base64 secret failed: %v", err)
	}

	// Verify with same base64-encoded secret
	claims, err := VerifyJWT(token, base64Secret)
	if err != nil {
		t.Fatalf("VerifyJWT with base64 secret failed: %v", err)
	}

	if claims.Subject != subject {
		t.Errorf("Subject mismatch: got %q, want %q", claims.Subject, subject)
	}
}
