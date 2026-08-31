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
	// message is parked on DeadLetterStream.
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

// RetryPolicy is the retry-then-dead-letter tuning the Router applies to
// every handler it registers.
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
// exponential backoff, then republishes it to DeadLetterStream with the
// reason recorded and acks it, so it stops cycling. Work that ran and
// produced a failure publishes a failure result event and returns nil; that
// never reaches retry or the dead-letter stream.
type StreamHandler func(ctx context.Context, event *Event) error

// SubscriberFactory opens a durable-stream subscriber bound to one consumer
// group and consumer identity. The Router asks for one per registered stream.
type SubscriberFactory func(group, consumer string) (message.Subscriber, error)

// Router consumes every durable stream in one process through a single
// watermill message.Router. The middleware stack, outermost first:
//
//   - Recoverer   — a panicking handler becomes an error rather than a
//     crashed subscriber goroutine.
//   - PoisonQueue — an error that survives Retry is republished to
//     DeadLetterStream with the reason in metadata, and the source message
//     is acked so it stops being redelivered.
//   - Retry       — a handler error is retried with exponential backoff up
//     to the policy's cap.
//
// One Router per process (docs/adr/0002, docs/adr/0006): the server runs
// one, each agent runs one.
type Router struct {
	router *message.Router
	newSub SubscriberFactory
	logger watermill.LoggerAdapter
}

// NewRouter builds a Router that republishes exhausted messages through
// deadLetterPublisher and opens a per-stream subscriber through newSub.
// Tests pass a watermill gochannel for both; NewRedisRouter wires the Redis
// Streams transport.
func NewRouter(
	deadLetterPublisher message.Publisher,
	newSub SubscriberFactory,
	policy RetryPolicy,
	logger watermill.LoggerAdapter,
) (*Router, error) {
	inner, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return nil, fmt.Errorf("eventbus: new router: %w", err)
	}

	poison, err := middleware.PoisonQueue(deadLetterPublisher, DeadLetterStream)
	if err != nil {
		return nil, fmt.Errorf("eventbus: poison queue: %w", err)
	}

	inner.AddMiddleware(
		middleware.Recoverer,
		poison,
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

// NewRedisRouter builds a Router backed by Redis Streams: the dead-letter
// republish and every per-stream subscriber use client. The dead-letter
// stream is capped at retention.MaxLenDefault so parked messages cannot grow
// without bound.
func NewRedisRouter(client redis.UniversalClient, policy RetryPolicy, retention RetentionPolicy, logger watermill.LoggerAdapter) (*Router, error) {
	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client:        client,
		DefaultMaxlen: retention.MaxLenDefault,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("eventbus: dead-letter publisher: %w", err)
	}

	newSub := func(group, consumer string) (message.Subscriber, error) {
		return redisstream.NewSubscriber(redisstream.SubscriberConfig{
			Client:        client,
			ConsumerGroup: group,
			Consumer:      consumer,
		}, logger)
	}

	return NewRouter(publisher, newSub, policy, logger)
}

// Handle registers handler as the sole consumer of stream, read as
// group/consumer. name is the handler's identity within the router and must
// be unique. Envelope decode lives here, so a listener is one registration
// call and never re-implements the decode-and-ack loop.
//
// A decode failure is returned as an error on purpose: a payload that will
// never parse is retried a few times (cheap, and covers a transient
// truncation) and then parked, rather than silently dropped.
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
