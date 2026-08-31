package eventbus

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// ErrNoResponder is returned by Request when no reply comes back — the
// timeout elapsed with nothing listening on the request channel or the
// responder too slow, or the reply subscription closed under it. On the
// timeout path (the common one) it also wraps the context's deadline error,
// so errors.Is(err, context.DeadlineExceeded) still matches there.
var ErrNoResponder = errors.New("eventbus: no responder answered the request")

// PubSubBus is the Redis Pub/Sub backed message queue. It carries fire-and-
// forget notifications and the one synchronous request/reply pattern on the
// bus (Request/Reply below) — the durable streams go through the Router
// instead. Request/Reply is the single implementation of that pattern: the
// heartbeat health check and the NFO-read call both use it.
type PubSubBus struct {
	client *redis.Client
}

// NewPubSubBus wraps client as a PubSubBus.
func NewPubSubBus(client *redis.Client) *PubSubBus {
	return &PubSubBus{client: client}
}

// ReplyChannel returns the per-request reply channel name for a correlation
// ID. Scoping the reply channel to the correlation ID lets many concurrent
// callers share the same request channel without stealing each other's
// replies.
func ReplyChannel(correlationID string) string {
	return fmt.Sprintf("reply.%s", correlationID)
}

// Publish marshals event to the canonical bus JSON form and publishes it on
// channel.
func (b *PubSubBus) Publish(ctx context.Context, channel string, event *Event) error {
	data, err := MarshalEvent(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, channel, data).Err()
}

// Subscribe returns a raw subscription to channel. Callers are responsible
// for closing it.
func (b *PubSubBus) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return b.client.Subscribe(ctx, channel)
}

// Request publishes event on requestChannel and blocks until a reply arrives
// on that correlation ID's reply channel, or ctx is done. It is the one
// synchronous call on the bus: the heartbeat health check and the NFO read
// both go through here.
//
// A timeout with no reply comes back as ErrNoResponder (wrapping the context
// error), so a caller can distinguish "the other end never answered" from a
// transport failure.
func (b *PubSubBus) Request(ctx context.Context, requestChannel string, event *Event) (*Event, error) {
	sub := b.client.Subscribe(ctx, ReplyChannel(event.GetCorrelationId()))
	defer func() { _ = sub.Close() }()

	// Wait for the subscription to be acknowledged before publishing, so
	// the request can't be picked up and replied to before we're listening.
	if _, err := sub.Receive(ctx); err != nil {
		return nil, err
	}

	if err := b.Publish(ctx, requestChannel, event); err != nil {
		return nil, err
	}

	select {
	case replyMsg, ok := <-sub.Channel():
		if !ok {
			return nil, fmt.Errorf("%w: reply subscription closed for correlation_id=%s", ErrNoResponder, event.GetCorrelationId())
		}
		var reply Event
		if err := UnmarshalEvent([]byte(replyMsg.Payload), &reply); err != nil {
			return nil, err
		}
		return &reply, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrNoResponder, ctx.Err())
	}
}

// Reply publishes event as the response to the request identified by
// correlationID. Used by listeners answering a Request call.
func (b *PubSubBus) Reply(ctx context.Context, correlationID string, event *Event) error {
	return b.Publish(ctx, ReplyChannel(correlationID), event)
}
