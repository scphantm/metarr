package services

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/server/auth"
	"Metarr/internal/server/handlers"
	"Metarr/internal/server/httpserver"
	"Metarr/internal/server/passwordhash"
	"Metarr/internal/server/session"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
)

// AuthServer implements metarrv1connect.AuthServiceHandler, ported directly
// from internal/server/handlers/auth.go — same session/passwordhash calls,
// only the transport changed.
type AuthServer struct {
	*handlers.Handlers
}

// AuthAuthPolicies is this service's method-name -> policy map. Login is
// the one RPC in the whole surface that needs no API key at all — matching
// the REST route, which is the only one registered outside protect(...).
var AuthAuthPolicies = map[string]httpserver.RPCPolicy{
	"Login":  {NoAuth: true},
	"Logout": {Group: auth.GroupConfig},
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

	apiKey, err := s.Sessions.Create(ctx)
	if err != nil {
		s.Logger.Error("failed to create session", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to create session"))
	}

	return connect.NewResponse(&metarrv1.AuthServiceLoginResponse{
		ApiKey:           apiKey,
		ExpiresInSeconds: int32(session.TTL.Seconds()),
	}), nil
}

func (s *AuthServer) Logout(
	ctx context.Context,
	req *connect.Request[metarrv1.AuthServiceLogoutRequest],
) (*connect.Response[metarrv1.AuthServiceLogoutResponse], error) {
	correlationID := correlation.FromContext(ctx)
	apiKey := auth.APIKeyFromContext(ctx)

	if err := s.Sessions.Delete(ctx, apiKey); err != nil {
		s.Logger.Error("failed to delete session", "correlation_id", correlationID, "error", err)
		return nil, connectError(http.StatusInternalServerError, errors.New("failed to log out"))
	}

	return connect.NewResponse(&metarrv1.AuthServiceLogoutResponse{Status: "logged_out"}), nil
}
