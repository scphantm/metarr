package services

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/appconfigstore"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/authsecret"
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
	// The request's name is carried into the token as its subject so an
	// issued token is traceable back to the integration it was minted for.
	if claims.Subject != "test-integration" {
		t.Fatalf("subject = %q, want %q", claims.Subject, "test-integration")
	}
}

// TestTokenServiceIssueToken_DefaultsSubjectWhenUnnamed verifies IssueToken
// still mints a usable token when the caller names no integration.
func TestTokenServiceIssueToken_DefaultsSubjectWhenUnnamed(t *testing.T) {
	secret := []byte("test-secret-32-bytes-for-hmac-sha256")
	withLiveConfig(t, &appconfig.Config{
		Auth: &appconfig.AuthConfig{HmacSecret: base64.StdEncoding.EncodeToString(secret)},
	})

	server := &TokenServer{Handlers: &handlers.Handlers{}}

	resp, err := server.IssueToken(context.Background(),
		connect.NewRequest(&metarrv1.IssueTokenRequest{
			Role:       metarrv1.AccessLevel_ACCESS_LEVEL_WEBHOOK,
			TtlSeconds: 3600,
		}))
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	claims, err := jwt.VerifyJWT(resp.Msg.JwtToken, secret)
	if err != nil {
		t.Fatalf("failed to verify JWT: %v", err)
	}
	if claims.Subject == "" {
		t.Fatal("expected a non-empty fallback subject")
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

// newTestTokenServer wires a TokenServer over an in-memory config store whose
// synchronous writes propagate straight to the live-config singleton, so a
// RotateHmacSecret call is observable both in the backend document and via
// authsecret on the next verify. Pair with withLiveConfig for singleton
// cleanup.
func newTestTokenServer(seed *appconfig.Config) (*TokenServer, *fakeConfigBackend) {
	backend := &fakeConfigBackend{cfg: seed}
	store := appconfigstore.New(backend, backend)
	store.SetPropagator(liveConfigPropagator{})
	return &TokenServer{Handlers: &handlers.Handlers{
		AppConfigStore: store,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}, backend
}

// RotateHmacSecret is admin-only, the same group IssueToken requires — a
// lower-privileged caller must not be able to invalidate every issued token.
func TestTokenAuthPolicies_RotateHmacSecretIsAdminOnly(t *testing.T) {
	policy, ok := TokenAuthPolicies["RotateHmacSecret"]
	if !ok {
		t.Fatal("no auth policy registered for RotateHmacSecret")
	}
	if policy.Group != auth.GroupConfig {
		t.Errorf("RotateHmacSecret group = %q, want %q", policy.Group, auth.GroupConfig)
	}
	if policy.Group != TokenAuthPolicies["IssueToken"].Group {
		t.Errorf("RotateHmacSecret group %q differs from IssueToken's %q",
			policy.Group, TokenAuthPolicies["IssueToken"].Group)
	}
	if policy.NoAuth || policy.ReadOnly {
		t.Errorf("RotateHmacSecret must not be NoAuth or ReadOnly: %+v", policy)
	}
}

// The core rotation contract: a fresh secret is generated server-side and
// stored through the normal config-store path, the stored value actually
// changes, and every token signed under the previous secret stops verifying
// the instant the write lands — with no raw secret anywhere in the response.
func TestTokenServiceRotateHmacSecret_ReplacesTheStoredSecretAndRevokesOldTokens(t *testing.T) {
	oldSecret := base64.StdEncoding.EncodeToString([]byte("old-secret-32-bytes-for-hmac-256"))
	seed := &appconfig.Config{Auth: &appconfig.AuthConfig{HmacSecret: oldSecret}}

	withLiveConfig(t, proto.Clone(seed).(*appconfig.Config))
	server, backend := newTestTokenServer(proto.Clone(seed).(*appconfig.Config))

	// A token minted under the pre-rotation secret verifies now.
	staleToken, err := authsecret.Sign("integration", string(jwt.RoleUser), 3600)
	if err != nil {
		t.Fatalf("authsecret.Sign: %v", err)
	}
	if _, err := authsecret.Verify(staleToken); err != nil {
		t.Fatalf("token should verify before rotation: %v", err)
	}

	resp, err := server.RotateHmacSecret(context.Background(),
		connect.NewRequest(&metarrv1.RotateHmacSecretRequest{}))
	if err != nil {
		t.Fatalf("RotateHmacSecret: %v", err)
	}

	// The stored secret changed, and the write went through the config store
	// (the backend document, not just the live singleton, carries it).
	stored := backend.cfg.GetAuth().GetHmacSecret()
	if stored == "" || stored == oldSecret {
		t.Fatalf("stored secret = %q, want a new non-empty value distinct from %q", stored, oldSecret)
	}
	// Same generator as bootstrap's hmacSecretSeedStep: 32 random bytes,
	// base64-encoded.
	raw, err := base64.StdEncoding.DecodeString(stored)
	if err != nil || len(raw) != 32 {
		t.Fatalf("rotated secret is not 32 base64-encoded bytes: len=%d err=%v", len(raw), err)
	}

	// The raw new value is never in the response: RotateHmacSecretResponse
	// has no fields, so it marshals to nothing.
	if wire, _ := proto.Marshal(resp.Msg); len(wire) != 0 {
		t.Errorf("RotateHmacSecretResponse carried %d bytes on the wire, want an empty message", len(wire))
	}

	// Rotation is immediate and total: the old token no longer verifies.
	if _, err := authsecret.Verify(staleToken); err == nil {
		t.Error("a token signed under the rotated-away secret still verifies")
	}
	// A token minted after rotation verifies against the new secret.
	freshToken, err := authsecret.Sign("integration", string(jwt.RoleUser), 3600)
	if err != nil {
		t.Fatalf("authsecret.Sign after rotation: %v", err)
	}
	if _, err := authsecret.Verify(freshToken); err != nil {
		t.Errorf("token signed under the new secret should verify: %v", err)
	}
}
