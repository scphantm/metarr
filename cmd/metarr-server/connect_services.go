package main

import (
	"net/http"

	"connectrpc.com/connect"

	"Metarr/internal/server/httpserver"
)

// newConnectService wraps one generated NewXServiceHandler(svc, opts...)
// constructor with this service's auth interceptor (plus any extra
// interceptors, e.g. Auth's per-method rate limiter) and returns it ready
// to append to the connectServices slice passed to httpserver.NewRouter.
// Every generated handler constructor has this same (svc, opts...) ->
// (path, handler) shape, so one generic helper covers every domain as it
// migrates off REST — see connect_auth.go's RPCPolicy and each
// services.XAuthPolicies map for what "this service's auth interceptor"
// means concretely.
//
// Connect runs the *first* interceptor in the list outermost (confirmed
// against connectrpc.com/connect's own newChain: "we usually wrap in
// reverse order to have the first interceptor from the slice act first").
// extraInterceptors go before the auth interceptor so they wrap outside
// it — matching the REST routes' own nesting, where login/logout were
// throttle(protect(...)): rate-limiting checked before the API key.
func newConnectService[T any](
	newHandler func(svc T, opts ...connect.HandlerOption) (string, http.Handler),
	svc T,
	policies map[string]httpserver.RPCPolicy,
	extraInterceptors ...connect.Interceptor,
) httpserver.ConnectService {
	interceptors := append(
		append([]connect.Interceptor{}, extraInterceptors...),
		httpserver.NewConnectAuthInterceptor(policies),
	)
	path, handler := newHandler(svc, connect.WithInterceptors(interceptors...))
	return httpserver.ConnectService{Path: path, Handler: handler}
}
