package eventbus

import (
	"context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// StreamBus is the publish side of the Redis Streams backed event bus, built
// on the watermill-redisstream library. Events published here are durable:
// they persist on the stream until a consumer group acknowledges them, so a
// listener that isn't running yet won't miss them. Consuming those streams
// is the Router's job (router.go).
type StreamBus struct {
	publisher *redisstream.Publisher
}

// NewStreamBus wraps client as a StreamBus, publishing through a
// watermill-redisstream Publisher configured with logger. Every publish sets
// the same approximate MAXLEN from policy.MaxLen — one cap for every stream
// (docs/adr/0006).
func NewStreamBus(client redis.UniversalClient, policy RetentionPolicy, logger watermill.LoggerAdapter) (*StreamBus, error) {
	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client:        client,
		DefaultMaxlen: policy.MaxLen,
	}, logger)
	if err != nil {
		return nil, err
	}

	return &StreamBus{publisher: publisher}, nil
}

// Publish appends event to topic's stream and returns immediately
// (non-blocking). topic is the same StreamTopic value the consumer registers
// with — stream name and consumer group bundled — so a publish and its
// listener cannot name different streams.
//
// It rejects a topic it must not append to: a pattern topic (a glob names
// many streams, not one) and a hand-built topic whose Name is not one the
// stream topic table resolves to. The topic constructors are the primary
// safety; this guard catches a StreamTopic assembled by hand.
//
// Consuming a durable stream is the Router's job (router.go), not this
// type's: StreamBus is the publish side only.
func (b *StreamBus) Publish(ctx context.Context, topic StreamTopic, event *Event) error {
	if err := streamTopicPublishable(topic); err != nil {
		return err
	}

	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}

	msg := message.NewMessage(watermill.NewUUID(), payload)
	msg.SetContext(ctx)
	return b.publisher.Publish(topic.Name, msg)
}

// Close releases resources owned by this bus. The watermill-redisstream
// publisher's only resource is the Redis client, and that client is shared
// and owned by whoever constructed this bus — not by the bus. Delegating to
// publisher.Close() here would call redis.Client.Close() on that shared
// client, cutting off every other user of it (the config store, the router,
// the presence watcher, the stats sampler). So this is deliberately a no-op.
func (b *StreamBus) Close() error {
	return nil
}
