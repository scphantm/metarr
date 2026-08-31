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
// an approximate MAXLEN from policy: HighVolumeStreams at MaxLenHigh, the
// rest at MaxLenDefault (docs/adr/0006).
func NewStreamBus(client redis.UniversalClient, policy RetentionPolicy, logger watermill.LoggerAdapter) (*StreamBus, error) {
	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{
		Client:        client,
		Maxlens:       policy.Maxlens(),
		DefaultMaxlen: policy.MaxLenDefault,
	}, logger)
	if err != nil {
		return nil, err
	}

	return &StreamBus{publisher: publisher}, nil
}

// Fire appends event to stream and returns immediately (non-blocking).
//
// Consuming a durable stream is the Router's job (router.go), not this
// type's: StreamBus is the publish side only.
func (b *StreamBus) Fire(ctx context.Context, stream string, event *Event) error {
	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}

	msg := message.NewMessage(watermill.NewUUID(), payload)
	msg.SetContext(ctx)
	return b.publisher.Publish(stream, msg)
}
