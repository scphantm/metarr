package services

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/jwt"
	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
)

// GetAuthScheme is the pre-login probe (docs/adr/0012): it reads the scheme
// from live config and succeeds with no credential on the context, so the UI
// can decide the render gate on a cold load.
func TestGetAuthScheme_ReturnsTheConfiguredScheme(t *testing.T) {
	for _, tc := range []struct {
		name string
		want metarrv1.AuthenticationScheme
	}{
		{"none", appconfig.AuthSchemeNone},
		{"password", appconfig.AuthSchemePassword},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withLiveConfig(t, &appconfig.Config{
				Admin: &appconfig.AdminUser{AuthenticationScheme: tc.want},
			})
			server := &AuthServer{Handlers: &handlers.Handlers{}}

			resp, err := server.GetAuthScheme(context.Background(),
				connect.NewRequest(&metarrv1.AuthServiceGetAuthSchemeRequest{}))
			if err != nil {
				t.Fatalf("GetAuthScheme: %v", err)
			}
			if resp.Msg.GetScheme() != tc.want {
				t.Fatalf("scheme = %v, want %v", resp.Msg.GetScheme(), tc.want)
			}
		})
	}
}

// A config that never named a scheme normalises to None on the way into live
// config, so the probe answers None rather than UNSPECIFIED.
func TestGetAuthScheme_NormalisesAnUnsetScheme(t *testing.T) {
	withLiveConfig(t, &appconfig.Config{})
	server := &AuthServer{Handlers: &handlers.Handlers{}}

	resp, err := server.GetAuthScheme(context.Background(),
		connect.NewRequest(&metarrv1.AuthServiceGetAuthSchemeRequest{}))
	if err != nil {
		t.Fatalf("GetAuthScheme: %v", err)
	}
	if resp.Msg.GetScheme() != appconfig.AuthSchemeNone {
		t.Fatalf("scheme = %v, want None", resp.Msg.GetScheme())
	}
}

// The probe is a NoAuth RPC: its policy entry must say so, since that is what
// lets the UI call it before any credential exists.
func TestAuthPolicies_GetAuthSchemeIsNoAuth(t *testing.T) {
	policy, ok := AuthAuthPolicies["GetAuthScheme"]
	if !ok {
		t.Fatal("no auth policy registered for GetAuthScheme")
	}
	if !policy.NoAuth {
		t.Errorf("GetAuthScheme policy = %+v, want NoAuth", policy)
	}
}

// TestAuthServiceLogin_SucceedsWithValidCredentials verifies that Login accepts
// valid username and password, returns a JWT token with admin role and correct TTL.
func TestAuthServiceLogin_SucceedsWithValidCredentials(t *testing.T) {
	password := "test-password"
	salt, hash, err := passwordhash.Hash(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	secret := []byte("test-secret-32-bytes-for-hmac-sha256")
	encodedSecret := base64.StdEncoding.EncodeToString(secret)

	withLiveConfig(t, &appconfig.Config{
		Admin: &appconfig.AdminUser{
			Username:             "admin",
			AuthenticationScheme: appconfig.AuthSchemePassword,
			PasswordSalt:         salt,
			PasswordHash:         hash,
		},
		Auth: &appconfig.AuthConfig{HmacSecret: encodedSecret},
	})

	server := &AuthServer{Handlers: &handlers.Handlers{}}

	resp, err := server.Login(context.Background(),
		connect.NewRequest(&metarrv1.AuthServiceLoginRequest{
			Username: "admin",
			Password: password,
		}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if resp.Msg.JwtToken == "" {
		t.Fatal("expected JWT token in response")
	}
	if resp.Msg.ExpiresAt == 0 {
		t.Fatal("expected expires_at in response")
	}

	claims, err := jwt.VerifyJWT(resp.Msg.JwtToken, secret)
	if err != nil {
		t.Fatalf("failed to verify JWT: %v", err)
	}
	if claims.Role != string(jwt.RoleAdmin) {
		t.Fatalf("role = %s, want admin", claims.Role)
	}
}

// TestAuthServiceLogin_FailsWithInvalidCredentials verifies that Login rejects
// invalid username or password.
func TestAuthServiceLogin_FailsWithInvalidCredentials(t *testing.T) {
	password := "test-password"
	salt, hash, err := passwordhash.Hash(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	secret := []byte("test-secret-32-bytes-for-hmac-sha256")
	encodedSecret := base64.StdEncoding.EncodeToString(secret)

	withLiveConfig(t, &appconfig.Config{
		Admin: &appconfig.AdminUser{
			Username:             "admin",
			AuthenticationScheme: appconfig.AuthSchemePassword,
			PasswordSalt:         salt,
			PasswordHash:         hash,
		},
		Auth: &appconfig.AuthConfig{HmacSecret: encodedSecret},
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &AuthServer{Handlers: &handlers.Handlers{Logger: logger}}

	resp, err := server.Login(context.Background(),
		connect.NewRequest(&metarrv1.AuthServiceLoginRequest{
			Username: "admin",
			Password: "wrong-password",
		}))
	if err == nil {
		t.Fatalf("Login with wrong password should fail, got response: %+v", resp)
	}
	if c := connect.CodeOf(err); c != connect.CodeUnauthenticated {
		t.Fatalf("expected Unauthenticated error, got %v", c)
	}
}

// TestTokenServiceIssueToken_SucceedsWithValidRequest verifies that IssueToken
// creates a valid JWT token with the specified role and TTL.
func TestTokenServiceIssueToken_SucceedsWithValidRequest(t *testing.T) {
	secret := []byte("test-secret-32-bytes-for-hmac-sha256")
	encodedSecret := base64.StdEncoding.EncodeToString(secret)

	withLiveConfig(t, &appconfig.Config{
		Auth: &appconfig.AuthConfig{HmacSecret: encodedSecret},
	})

	server := &TokenServer{Handlers: &handlers.Handlers{}}

	resp, err := server.IssueToken(context.Background(),
		connect.NewRequest(&metarrv1.IssueTokenRequest{
			Role:       metarrv1.AccessLevel_ACCESS_LEVEL_USER,
			TtlSeconds: 3600,
			Name:       "test-integration",
		}))
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if resp.Msg.JwtToken == "" {
		t.Fatal("expected JWT token in response")
	}
	if resp.Msg.ExpiresAt == 0 {
		t.Fatal("expected expires_at in response")
	}

	claims, err := jwt.VerifyJWT(resp.Msg.JwtToken, secret)
	if err != nil {
		t.Fatalf("failed to verify JWT: %v", err)
	}
	if claims.Role != string(jwt.RoleUser) {
		t.Fatalf("role = %s, want user", claims.Role)
	}
}

// TestTokenServiceIssueToken_FailsWithInvalidRole verifies that IssueToken
// rejects an unspecified role.
func TestTokenServiceIssueToken_FailsWithUnspecifiedRole(t *testing.T) {
	secret := []byte("test-secret-32-bytes-for-hmac-sha256")
	encodedSecret := base64.StdEncoding.EncodeToString(secret)

	withLiveConfig(t, &appconfig.Config{
		Auth: &appconfig.AuthConfig{HmacSecret: encodedSecret},
	})

	server := &TokenServer{Handlers: &handlers.Handlers{}}

	resp, err := server.IssueToken(context.Background(),
		connect.NewRequest(&metarrv1.IssueTokenRequest{
			Role:       metarrv1.AccessLevel_ACCESS_LEVEL_UNSPECIFIED,
			TtlSeconds: 3600,
		}))
	if err == nil {
		t.Fatalf("IssueToken with unspecified role should fail, got response: %+v", resp)
	}
	if c := connect.CodeOf(err); c != connect.CodeInvalidArgument {
		t.Fatalf("expected InvalidArgument error, got %v", c)
	}
}
