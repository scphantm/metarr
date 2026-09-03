package httpserver

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"Metarr/internal/server/auth"
	"Metarr/internal/server/session"
	"Metarr/internal/shared/appconfig"
)

// schemeNoneSyntheticKey is the API-key marker attached to a request's
// context when the authentication scheme is None. It stands in for a real
// resolved key so downstream code (auth.APIKeyFromContext, audit logging)
// has a non-empty value, and is deliberately not a value auth.Resolve or the
// session store would ever mint (docs/adr/0012).
const schemeNoneSyntheticKey = "authentication-scheme-none"

// RPCPolicy is a Connect RPC's auth requirement — the gRPC-Web equivalent of
// a REST route's (auth.Group, HTTP method) pair passed to protect() in
// router.go. gRPC has no HTTP verb, so the read-only role's GET-only
// restriction (see auth.Authorized) becomes this explicit ReadOnly flag
// instead of being inferred from the method.
type RPCPolicy struct {
	Group auth.Group
	// ReadOnly marks an RPC as safe for auth.RoleReadOnly, matching how that
	// role is restricted to GET today.
	ReadOnly bool
	// NoAuth marks an RPC callable without any API key at all — currently
	// only Login.
	NoAuth bool
}

// connectAuthInterceptor authenticates and authorizes each RPC the same way
// requireAPIKey does for REST: resolve the caller's role from an API key
// (session key first, then the static config-based categories), then check
// that role against the RPC's declared policy. One instance is constructed
// per service with that service's own method-name -> policy map, so each
// service's auth requirements live next to its own implementation.
type connectAuthInterceptor struct {
	sessions *session.Store
	policies map[string]RPCPolicy
}

// NewConnectAuthInterceptor builds an interceptor for one service. policies
// is keyed by RPC method name (e.g. "List", "Upsert" — the last segment of
// connect.Spec.Procedure), not the fully-qualified procedure string.
func NewConnectAuthInterceptor(sessions *session.Store, policies map[string]RPCPolicy) connect.Interceptor {
	return &connectAuthInterceptor{sessions: sessions, policies: policies}
}

func (i *connectAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, err := i.authorize(ctx, req.Spec().Procedure, req.Header())
		if err != nil {
			return nil, err
		}
		return next(ctx, req)
	}
}

func (i *connectAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	// This interceptor is only ever installed on server-side handlers, never
	// on an outgoing client, so there's nothing for this half to do.
	return next
}

func (i *connectAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx, err := i.authorize(ctx, conn.Spec().Procedure, conn.RequestHeader())
		if err != nil {
			return err
		}
		return next(ctx, conn)
	}
}

// authorize implements the actual check, shared by unary and streaming
// RPCs alike — a streaming RPC is authorized once at call start, not
// re-checked per message, matching how each topic's own server-streaming
// RPC replaces wsbus's per-subscribe re-authorization (there is no more
// "resubscribe to a different topic over one shared connection" case to
// re-check).
func (i *connectAuthInterceptor) authorize(ctx context.Context, procedure string, header http.Header) (context.Context, error) {
	policy, ok := i.policies[methodName(procedure)]
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, errors.New("no auth policy registered for this RPC"))
	}
	if policy.NoAuth {
		return ctx, nil
	}

	// Authentication scheme None: every request runs as the administrator
	// (docs/adr/0012). Attach an administrator role and a synthetic key
	// marker and return allowed — no header is read, no session store is
	// consulted, no API key is resolved. A presented X-Api-Key is ignored.
	if appconfig.Get().GetAdmin().GetAuthenticationScheme() != appconfig.AuthSchemePassword {
		ctx = auth.WithAPIKey(ctx, schemeNoneSyntheticKey)
		ctx = auth.WithRole(ctx, auth.RoleAdmin)
		return ctx, nil
	}

	apiKey := header.Get(apiKeyHeaderName)

	role := auth.RoleAdmin
	if !i.sessions.Valid(ctx, apiKey) {
		resolvedRole, ok := auth.Resolve(appconfig.Get(), apiKey)
		if !ok {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing or invalid API key"))
		}
		role = resolvedRole
	}

	method := http.MethodPost
	if policy.ReadOnly {
		method = http.MethodGet
	}
	if !auth.Authorized(role, policy.Group, method) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("API key not authorized for this endpoint"))
	}

	ctx = auth.WithAPIKey(ctx, apiKey)
	ctx = auth.WithRole(ctx, role)
	return ctx, nil
}

// methodName extracts the RPC method from a Connect procedure string
// ("/metarr.v1.SonarrInterfaceService/List" -> "List").
func methodName(procedure string) string {
	idx := strings.LastIndexByte(procedure, '/')
	if idx == -1 {
		return procedure
	}
	return procedure[idx+1:]
}
