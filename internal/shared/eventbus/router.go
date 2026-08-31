package eventbus

import (
	"context"
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/redis/go-redis/v9"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// Retry policy defaults.
//
// These are the sane hardcoded values this slice ships with. The event_bus
// config section (docs/adr/0006) moves them to live configuration and feeds
// them in through a RetryPolicy value instead; nothing else about the Router
// changes when that happens.
const (
	// DefaultRetryAttempts is the number of retries after the first attempt,
	// so a handler runs at most DefaultRetryAttempts+1 times before its
	// message is logged at error level and acked (dropped).
	DefaultRetryAttempts = 4
	// DefaultRetryBackoffBase is the wait before the first retry.
	DefaultRetryBackoffBase = 500 * time.Millisecond
	// DefaultRetryBackoffMax caps the exponential backoff between retries.
	DefaultRetryBackoffMax = 30 * time.Second

	// retryBackoffMultiplier is the factor each successive backoff is scaled
	// by. Not tunable: doubling is the conventional choice and the config
	// section deliberately exposes only base and max.
	retryBackoffMultiplier = 2.0
)

// RetryPolicy is the retry tuning the Router applies to every handler it
// registers. A message whose error survives every retry is logged and
// acked; there is no dead-letter stream.
type RetryPolicy struct {
	// MaxAttempts is the number of retries after the first attempt.
	MaxAttempts int
	// BackoffBase is the wait before the first retry.
	BackoffBase time.Duration
	// BackoffMax caps the exponential backoff.
	BackoffMax time.Duration
}

// DefaultRetryPolicy returns the built-in policy. It is the reference the
// event_bus section's built-in defaults (builtin_defaults.json) must agree
// with, and what a process with no live config to read — the agent — runs
// with.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: DefaultRetryAttempts,
		BackoffBase: DefaultRetryBackoffBase,
		BackoffMax:  DefaultRetryBackoffMax,
	}
}

// RetryPolicyFromConfig builds a RetryPolicy from the live event_bus config
// section. The server reads its Router policy this way instead of from the
// constants above (docs/adr/0006).
func RetryPolicyFromConfig(c *metarrv1.EventBusConfig) RetryPolicy {
	return RetryPolicy{
		MaxAttempts: int(c.GetRetryAttempts()),
		BackoffBase: time.Duration(c.GetRetryBackoffBaseMs()) * time.Millisecond,
		BackoffMax:  time.Duration(c.GetRetryBackoffMaxMs()) * time.Millisecond,
	}
}

// StreamHandler is what a durable-stream listener registers with the Router.
// It receives the already-decoded envelope.
//
// Failure convention (documented, not enforced): return an error only when
// the message could not be processed at all — an undecodable payload, a
// datastore that is unreachable. The Router retries such a message with
// exponential backoff; once the retries are spent it logs the message at
// error level with its identifier and acks it (dropped), so one poison
// message stops cycling instead of stalling its consumer group. Work that
// ran and produced a failure publishes a failure result event and returns
// nil; that never reaches the retry path.
type StreamHandler func(ctx context.Context, event *Event) error

// SubscriberFactory opens a durable-stream subscriber bound to one consumer
// group and consumer identity. The Router asks for one per registered stream.
type SubscriberFactory func(group, consumer string) (message.Subscriber, error)

// Router consumes every durable stream in one process through a single
// watermill message.Router. The middleware stack, outermost first:
//
//   - Recoverer      — a panicking handler becomes an error rather than a
//     crashed subscriber goroutine.
//   - dropAfterRetry — an error that survives Retry is logged at error level
//     with the message identifier and the source message is acked, so a
//     poison message stops being redelivered instead of stalling its
//     consumer group. There is no parking stream; the log line is the
//     record.
//   - Retry          — a handler error is retried with exponential backoff
//     up to the policy's cap.
//
// One Router per process (docs/adr/0002, docs/adr/0006): the server runs
// one, each agent runs one.
type Router struct {
	router *message.Router
	newSub SubscriberFactory
	logger watermill.LoggerAdapter
}

// NewRouter builds a Router that opens a per-stream subscriber through
// newSub. Tests pass a watermill gochannel; NewRedisRouter wires the Redis
// Streams transport.
func NewRouter(
	newSub SubscriberFactory,
	policy RetryPolicy,
	logger watermill.LoggerAdapter,
) (*Router, error) {
	inner, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return nil, fmt.Errorf("eventbus: new router: %w", err)
	}

	inner.AddMiddleware(
		middleware.Recoverer,
		dropAfterRetry(logger),
		middleware.Retry{
			MaxRetries:      policy.MaxAttempts,
			InitialInterval: policy.BackoffBase,
			MaxInterval:     policy.BackoffMax,
			Multiplier:      retryBackoffMultiplier,
			Logger:          logger,
		}.Middleware,
	)

	return &Router{router: inner, newSub: newSub, logger: logger}, nil
}

// dropAfterRetry stands in for a dead-letter stream. Ordered outside Retry,
// so it runs once every retry is spent: it logs the still-failing message at
// error level with its identifier and returns success, so the message is
// acked (dropped) rather than parked or redelivered forever.
func dropAfterRetry(logger watermill.LoggerAdapter) message.HandlerMiddleware {
	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			produced, err := next(msg)
			if err != nil {
				logger.Error("eventbus: dropping message after retries exhausted", err, watermill.LogFields{
					"message_uuid": msg.UUID,
				})
				return nil, nil
			}
			return produced, nil
		}
	}
}

// NewRedisRouter builds a Router backed by Redis Streams: every per-stream
// subscriber uses client.
func NewRedisRouter(client redis.UniversalClient, policy RetryPolicy, logger watermill.LoggerAdapter) (*Router, error) {
	newSub := func(group, consumer string) (message.Subscriber, error) {
		return redisstream.NewSubscriber(redisstream.SubscriberConfig{
			Client:        client,
			ConsumerGroup: group,
			Consumer:      consumer,
		}, logger)
	}

	return NewRouter(newSub, policy, logger)
}

// Handle registers handler as the sole consumer of stream, read as
// group/consumer. name is the handler's identity within the router and must
// be unique. Envelope decode lives here, so a listener is one registration
// call and never re-implements the decode-and-ack loop.
//
// A decode failure is returned as an error on purpose: a payload that will
// never parse is retried a few times (cheap, and covers a transient
// truncation) and then logged and dropped, rather than silently swallowed.
func (r *Router) Handle(name, stream, group, consumer string, handler StreamHandler) error {
	subscriber, err := r.newSub(group, consumer)
	if err != nil {
		return fmt.Errorf("eventbus: subscriber for %s: %w", stream, err)
	}

	r.router.AddNoPublisherHandler(name, stream, subscriber, func(msg *message.Message) error {
		var event Event
		if err := UnmarshalEvent(msg.Payload, &event); err != nil {
			return fmt.Errorf("eventbus: decode envelope on %s: %w", stream, err)
		}
		return handler(msg.Context(), &event)
	})
	return nil
}

// Run starts every registered handler and blocks until ctx is cancelled or
// the router stops on its own.
func (r *Router) Run(ctx context.Context) error {
	return r.router.Run(ctx)
}

// Running is closed once the router has started every handler. Callers that
// must publish only after the consumers are live — tests, and startup code
// that fires a warm-up event — wait on it.
func (r *Router) Running() chan struct{} {
	return r.router.Running()
}

// Close stops the router and closes its subscribers.
func (r *Router) Close() error {
	return r.router.Close()
}
