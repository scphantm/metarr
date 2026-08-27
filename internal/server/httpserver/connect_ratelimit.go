package httpserver

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
)

// methodRateLimit is one RPC's independent call budget — its own counter, so
// throttling one method never steals budget from another. Mirrors
// rateLimit's per-route independent-counter behavior in middleware.go.
type methodRateLimit struct {
	interval time.Duration
	lastNano atomic.Int64
}

func (m *methodRateLimit) allow() bool {
	now := time.Now().UnixNano()
	last := m.lastNano.Load()
	if now-last < int64(m.interval) {
		return false
	}
	return m.lastNano.CompareAndSwap(last, now)
}

// connectRateLimitInterceptor throttles specific RPC methods within a
// service, keyed by method name (the same "List"/"Upsert"/... short name
// RPCPolicy uses) — a method with no entry in limits is never throttled.
type connectRateLimitInterceptor struct {
	limits map[string]*methodRateLimit
}

// NewConnectRateLimitInterceptor builds a rate limiter for one service.
// intervals is keyed by RPC method name; each named method gets its own
// independent budget, matching how the REST login/logout routes each get
// their own throttle(...) wrapper today rather than sharing one counter.
func NewConnectRateLimitInterceptor(intervals map[string]time.Duration) connect.Interceptor {
	limits := make(map[string]*methodRateLimit, len(intervals))
	for method, interval := range intervals {
		limits[method] = &methodRateLimit{interval: interval}
	}
	return &connectRateLimitInterceptor{limits: limits}
}

func (i *connectRateLimitInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !i.allow(req.Spec().Procedure) {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("too many requests"))
		}
		return next(ctx, req)
	}
}

func (i *connectRateLimitInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *connectRateLimitInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		if !i.allow(conn.Spec().Procedure) {
			return connect.NewError(connect.CodeResourceExhausted, errors.New("too many requests"))
		}
		return next(ctx, conn)
	}
}

func (i *connectRateLimitInterceptor) allow(procedure string) bool {
	limit, ok := i.limits[methodName(procedure)]
	if !ok {
		return true
	}
	return limit.allow()
}
