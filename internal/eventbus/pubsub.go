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

func (b *PubSubBus) Publish(ctx context.Context, channel string, evt Event) error {
	data, err := json.Marshal(evt)
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

// Request publishes evt to requestChannel and blocks until a reply arrives
// on that correlation ID's reply channel, or ctx is done. This backs the
// heartbeat API's blocking behavior.
func (b *PubSubBus) Request(ctx context.Context, requestChannel string, evt Event) (Event, error) {
	sub := b.client.Subscribe(ctx, ReplyChannel(evt.CorrelationID))
	defer sub.Close()

	// Wait for the subscription to be acknowledged before publishing, so
	// the request can't be picked up and replied to before we're listening.
	if _, err := sub.Receive(ctx); err != nil {
		return Event{}, err
	}

	if err := b.Publish(ctx, requestChannel, evt); err != nil {
		return Event{}, err
	}

	select {
	case msg, ok := <-sub.Channel():
		if !ok {
			return Event{}, fmt.Errorf("eventbus: reply subscription closed for correlation_id=%s", evt.CorrelationID)
		}
		var reply Event
		if err := json.Unmarshal([]byte(msg.Payload), &reply); err != nil {
			return Event{}, err
		}
		return reply, nil
	case <-ctx.Done():
		return Event{}, ctx.Err()
	}
}

// Reply publishes evt as the response to the request identified by
// correlationID. Used by listeners answering a Request call.
func (b *PubSubBus) Reply(ctx context.Context, correlationID string, evt Event) error {
	return b.Publish(ctx, ReplyChannel(correlationID), evt)
}
