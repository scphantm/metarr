package eventbus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// PubSubBus is the Redis Pub/Sub backed message queue used for the
// heartbeat's synchronous request/reply exchange.
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

// Publish marshals event to JSON and publishes it on channel.
func (b *PubSubBus) Publish(ctx context.Context, channel string, event Event) error {
	data, err := json.Marshal(event)
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

// Request publishes event to requestChannel and blocks until a reply arrives
// on that correlation ID's reply channel, or ctx is done. This backs the
// heartbeat API's blocking behavior.
func (b *PubSubBus) Request(ctx context.Context, requestChannel string, event Event) (Event, error) {
	sub := b.client.Subscribe(ctx, ReplyChannel(event.CorrelationID))
	defer sub.Close()

	// Wait for the subscription to be acknowledged before publishing, so
	// the request can't be picked up and replied to before we're listening.
	if _, err := sub.Receive(ctx); err != nil {
		return Event{}, err
	}

	if err := b.Publish(ctx, requestChannel, event); err != nil {
		return Event{}, err
	}

	select {
	case replyMsg, ok := <-sub.Channel():
		if !ok {
			return Event{}, fmt.Errorf("eventbus: reply subscription closed for correlation_id=%s", event.CorrelationID)
		}
		var reply Event
		if err := json.Unmarshal([]byte(replyMsg.Payload), &reply); err != nil {
			return Event{}, err
		}
		return reply, nil
	case <-ctx.Done():
		return Event{}, ctx.Err()
	}
}

// Reply publishes event as the response to the request identified by
// correlationID. Used by listeners answering a Request call.
func (b *PubSubBus) Reply(ctx context.Context, correlationID string, event Event) error {
	return b.Publish(ctx, ReplyChannel(correlationID), event)
}
