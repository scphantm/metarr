package httpserver

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"connectrpc.com/connect"

	"Metarr/internal/server/auth"
	"Metarr/internal/server/jwt"
	"Metarr/internal/shared/appconfig"
)

// withLiveConfig arranges cfg as the process-wide live config for one test,
// Normalized as production does, and restores the previous value afterwards.
func withLiveConfig(t *testing.T, cfg *appconfig.Config) {
	t.Helper()
	previous := appconfig.Get()
	appconfig.Set(appconfig.Normalize(cfg))
	t.Cleanup(func() { appconfig.Set(previous) })
}

// testInterceptor builds the real interceptor and returns it as its concrete
// type so the test can drive authorize() directly.
func testInterceptor(t *testing.T, policies map[string]RPCPolicy) *connectAuthInterceptor {
	t.Helper()
	return NewConnectAuthInterceptor(policies).(*connectAuthInterceptor)
}

const testProcedure = "/metarr.v1.SomeService/Do"

var oneWritePolicy = map[string]RPCPolicy{"Do": {Group: auth.GroupConfig}}

const testHmacSecret = "test-secret-32-bytes-for-hmac-sha256"

func schemeConfig(scheme appconfig.AuthenticationScheme) *appconfig.Config {
	return &appconfig.Config{
		Admin: &appconfig.AdminUser{AuthenticationScheme: scheme},
		Auth: &appconfig.AuthConfig{
			HmacSecret: base64.StdEncoding.EncodeToString([]byte(testHmacSecret)),
		},
	}
}

func createJWT(t *testing.T, subject string, role string) string {
	t.Helper()
	token, err := jwt.SignJWT(subject, role, 3600, []byte(testHmacSecret))
	if err != nil {
		t.Fatalf("failed to create JWT: %v", err)
	}
	return token
}

func TestAuthorize_SchemeNone_NoKey_IsAdministrator(t *testing.T) {
	withLiveConfig(t, schemeConfig(appconfig.AuthSchemeNone))
	i := testInterceptor(t, oneWritePolicy)

	ctx, err := i.authorize(context.Background(), testProcedure, http.Header{})
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if role, ok := auth.RoleFromContext(ctx); !ok || role != auth.RoleAdmin {
		t.Fatalf("context role = %v (ok=%v), want administrator", role, ok)
	}
	if got := auth.APIKeyFromContext(ctx); got != schemeNoneSyntheticKey {
		t.Fatalf("context api key = %q, want the synthetic marker", got)
	}
}

func TestAuthorize_SchemeNone_InvalidJWT_StillAdministrator(t *testing.T) {
	withLiveConfig(t, schemeConfig(appconfig.AuthSchemeNone))
	i := testInterceptor(t, oneWritePolicy)

	// An invalid JWT is presented; scheme None ignores it and synthesizes admin.
	header := http.Header{}
	header.Set(apiKeyHeaderName, "invalid-jwt-token")

	ctx, err := i.authorize(context.Background(), testProcedure, header)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if role, ok := auth.RoleFromContext(ctx); !ok || role != auth.RoleAdmin {
		t.Fatalf("context role = %v (ok=%v), want administrator", role, ok)
	}
}

func TestAuthorize_SchemePassword_NoKey_IsUnauthenticated(t *testing.T) {
	withLiveConfig(t, schemeConfig(appconfig.AuthSchemePassword))
	i := testInterceptor(t, oneWritePolicy)

	_, err := i.authorize(context.Background(), testProcedure, http.Header{})
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}
}

func TestAuthorize_SchemePassword_ValidAdminJWT_IsAllowed(t *testing.T) {
	withLiveConfig(t, schemeConfig(appconfig.AuthSchemePassword))
	i := testInterceptor(t, oneWritePolicy)

	token := createJWT(t, "admin", string(jwt.RoleAdmin))
	header := http.Header{}
	header.Set(apiKeyHeaderName, token)

	ctx, err := i.authorize(context.Background(), testProcedure, header)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if role, ok := auth.RoleFromContext(ctx); !ok || role != auth.RoleAdmin {
		t.Fatalf("context role = %v (ok=%v), want administrator", role, ok)
	}
}

func TestAuthorize_SchemePassword_ValidUserJWT_IsAllowed(t *testing.T) {
	withLiveConfig(t, schemeConfig(appconfig.AuthSchemePassword))
	// Use a policy that user role can access
	tasksPolicy := map[string]RPCPolicy{"Do": {Group: auth.GroupTasks}}
	i := NewConnectAuthInterceptor(tasksPolicy).(*connectAuthInterceptor)

	token := createJWT(t, "integration", string(jwt.RoleUser))
	header := http.Header{}
	header.Set(apiKeyHeaderName, token)

	ctx, err := i.authorize(context.Background(), testProcedure, header)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if role, ok := auth.RoleFromContext(ctx); !ok || role != auth.RoleUser {
		t.Fatalf("context role = %v (ok=%v), want user", role, ok)
	}
}

func TestAuthorize_SchemePassword_InvalidJWT_IsUnauthenticated(t *testing.T) {
	withLiveConfig(t, schemeConfig(appconfig.AuthSchemePassword))
	i := testInterceptor(t, oneWritePolicy)

	header := http.Header{}
	header.Set(apiKeyHeaderName, "invalid-jwt-token")

	_, err := i.authorize(context.Background(), testProcedure, header)
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", connect.CodeOf(err))
	}
}

// decodeHMACSecret memoises the base64 decode across authorize() calls; it
// must still return the right bytes when the encoded secret changes (a
// rotation) and when it does not.
func TestDecodeHMACSecret_CachesAndRefreshesOnChange(t *testing.T) {
	first := base64.StdEncoding.EncodeToString([]byte("secret-one"))
	second := base64.StdEncoding.EncodeToString([]byte("secret-two"))

	got, err := decodeHMACSecret(first)
	if err != nil || string(got) != "secret-one" {
		t.Fatalf("decodeHMACSecret(first) = %q, %v; want \"secret-one\", nil", got, err)
	}
	// Same input again: the cached slice comes back.
	again, _ := decodeHMACSecret(first)
	if string(again) != "secret-one" {
		t.Fatalf("cached decode = %q, want \"secret-one\"", again)
	}
	// A rotated secret must not keep returning the stale bytes.
	rotated, err := decodeHMACSecret(second)
	if err != nil || string(rotated) != "secret-two" {
		t.Fatalf("decodeHMACSecret(second) = %q, %v; want \"secret-two\", nil", rotated, err)
	}
	if _, err := decodeHMACSecret("not-base64!!!"); err == nil {
		t.Fatal("expected an error decoding invalid base64")
	}
}

// The missing-policy guard is unchanged by the scheme early-out: it still
// runs first, so an RPC with no registered policy is CodeInternal even under
// scheme None.
func TestAuthorize_MissingPolicy_IsInternal_UnderEitherScheme(t *testing.T) {
	for _, scheme := range []appconfig.AuthenticationScheme{
		appconfig.AuthSchemeNone, appconfig.AuthSchemePassword,
	} {
		withLiveConfig(t, schemeConfig(scheme))
		i := testInterceptor(t, oneWritePolicy)

		_, err := i.authorize(context.Background(), "/metarr.v1.SomeService/Unregistered", http.Header{})
		if connect.CodeOf(err) != connect.CodeInternal {
			t.Fatalf("scheme %v: code = %v, want Internal", scheme, connect.CodeOf(err))
		}
	}
}
