package services

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/jwt"
	"Metarr/internal/server/passwordhash"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// AuthServer implements metarrv1connect.AuthServiceHandler: password login
// that mints a session JWT, a client-side logout, and the pre-login scheme
// probe.
type AuthServer struct {
	*handlers.Handlers
}

// AuthAuthPolicies is this service's method-name -> policy map. Login and
// GetAuthScheme need no JWT at all: Login mints the session token, and
// GetAuthScheme is the pre-login probe the UI calls on a cold load to decide
// whether to show the login gate (docs/adr/0012).
var AuthAuthPolicies = map[string]httpserver.RPCPolicy{
	"Login":         {NoAuth: true},
	"Logout":        {Group: auth.GroupConfig},
	"GetAuthScheme": {NoAuth: true},
}

func (s *AuthServer) Login(
	ctx context.Context,
	req *connect.Request[metarrv1.AuthServiceLoginRequest],
) (*connect.Response[metarrv1.AuthServiceLoginResponse], error) {
	correlationID := correlation.FromContext(ctx)

	admin := appconfig.Get().Admin
	username := req.Msg.GetUsername()
	if admin.Username == "" || username == "" || username != admin.Username ||
		!passwordhash.Verify(req.Msg.GetPassword(), admin.PasswordSalt, admin.PasswordHash) {
		s.Logger.Warn("login failed", "correlation_id", correlationID, "username", username)
		return nil, connectError(http.StatusUnauthorized, errors.New("invalid username or password"))
	}

	cfg := appconfig.Get()
	if cfg.Auth == nil || cfg.Auth.HmacSecret == "" {
		s.Logger.Error("hmac secret not configured", "correlation_id", correlationID)
		return nil, connectError(http.StatusInternalServerError, errors.New("authentication not properly configured"))
	}

	secret, err := base64.StdEncoding.DecodeString(cfg.Auth.HmacSecret)
	if err != nil {
		s.Logger.Error("failed to decode hmac secret", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("authentication configuration error"))
	}

	const sessionTTL = 24 * 60 * 60
	token, err := jwt.SignJWT(username, string(jwt.RoleAdmin), int32(sessionTTL), secret)
	if err != nil {
		s.Logger.Error("failed to create JWT", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to create session token"))
	}

	expiresAt := time.Now().Unix() + sessionTTL

	return connect.NewResponse(&metarrv1.AuthServiceLoginResponse{
		JwtToken:  token,
		ExpiresAt: expiresAt,
	}), nil
}

// GetAuthScheme returns the active authentication scheme and nothing else.
// It carries a NoAuth policy, so it answers before any credential exists —
// the UI's cold-load probe for whether to show the login gate
// (docs/adr/0012). The scheme is read from live config, normalised, so the
// answer is never AUTHENTICATION_SCHEME_UNSPECIFIED.
func (s *AuthServer) GetAuthScheme(
	ctx context.Context,
	req *connect.Request[metarrv1.AuthServiceGetAuthSchemeRequest],
) (*connect.Response[metarrv1.AuthServiceGetAuthSchemeResponse], error) {
	return connect.NewResponse(&metarrv1.AuthServiceGetAuthSchemeResponse{
		Scheme: appconfig.Get().GetAdmin().GetAuthenticationScheme(),
	}), nil
}

// Logout is a client-side operation: JWTs are stateless, so there is no
// server-side session to end — the client discards its token. This endpoint
// stays so the UI has a call to make on logout and so a future audit-log
// hook has somewhere to live.
func (s *AuthServer) Logout(
	ctx context.Context,
	req *connect.Request[metarrv1.AuthServiceLogoutRequest],
) (*connect.Response[metarrv1.AuthServiceLogoutResponse], error) {
	return connect.NewResponse(&metarrv1.AuthServiceLogoutResponse{Status: "logged_out"}), nil
}

// TokenServer implements metarrv1connect.TokenServiceHandler.
type TokenServer struct {
	*handlers.Handlers
}

// TokenAuthPolicies defines the auth policy for TokenService.IssueToken.
// IssueToken is admin-only.
var TokenAuthPolicies = map[string]httpserver.RPCPolicy{
	"IssueToken": {Group: auth.GroupConfig},
}

// IssueToken creates a new JWT token with the specified role and TTL.
// Only callable by admin users.
func (s *TokenServer) IssueToken(
	ctx context.Context,
	req *connect.Request[metarrv1.IssueTokenRequest],
) (*connect.Response[metarrv1.IssueTokenResponse], error) {
	correlationID := correlation.FromContext(ctx)

	role := req.Msg.GetRole()
	if role == metarrv1.AccessLevel_ACCESS_LEVEL_UNSPECIFIED {
		return nil, connectError(http.StatusBadRequest, errors.New("role must be specified"))
	}

	ttl := req.Msg.GetTtlSeconds()
	if ttl <= 0 {
		return nil, connectError(http.StatusBadRequest, errors.New("ttl_seconds must be positive"))
	}

	const maxTTL = 365 * 24 * 60 * 60
	if ttl > maxTTL {
		return nil, connectError(http.StatusBadRequest, errors.New("ttl_seconds exceeds maximum (365 days)"))
	}

	cfg := appconfig.Get()
	if cfg.Auth == nil || cfg.Auth.HmacSecret == "" {
		s.Logger.Error("hmac secret not configured", "correlation_id", correlationID)
		return nil, connectError(http.StatusInternalServerError, errors.New("authentication not properly configured"))
	}

	secret, err := base64.StdEncoding.DecodeString(cfg.Auth.HmacSecret)
	if err != nil {
		s.Logger.Error("failed to decode hmac secret", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("authentication configuration error"))
	}

	// name identifies the integration the token is for; it becomes the JWT
	// subject so an issued token is traceable back to who asked for it.
	// Falls back to a generic label when the caller names nothing.
	subject := req.Msg.GetName()
	if subject == "" {
		subject = "integration"
	}

	roleStr := roleFromAccessLevel(role)
	token, err := jwt.SignJWT(subject, roleStr, ttl, secret)
	if err != nil {
		s.Logger.Error("failed to create JWT", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to create token"))
	}

	expiresAt := time.Now().Unix() + int64(ttl)

	return connect.NewResponse(&metarrv1.IssueTokenResponse{
		JwtToken:  token,
		ExpiresAt: expiresAt,
	}), nil
}

// roleFromAccessLevel converts proto AccessLevel to jwt role string.
func roleFromAccessLevel(level metarrv1.AccessLevel) string {
	switch level {
	case metarrv1.AccessLevel_ACCESS_LEVEL_ADMIN:
		return string(jwt.RoleAdmin)
	case metarrv1.AccessLevel_ACCESS_LEVEL_USER:
		return string(jwt.RoleUser)
	case metarrv1.AccessLevel_ACCESS_LEVEL_WEBHOOK:
		return string(jwt.RoleWebhook)
	case metarrv1.AccessLevel_ACCESS_LEVEL_READ_ONLY:
		return string(jwt.RoleReadOnly)
	default:
		return ""
	}
}
