package eventbus

import (
	"context"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
)

// StreamBus is the Redis Streams backed event-driven backend, built on the
// watermill-redisstream library. Unlike PubSubBus, events published here
// are durable: they persist on the stream until acknowledged, so a listener
// that isn't running yet won't miss them. Consumer group creation, claiming
// stuck pending messages, and acknowledgement are all handled by the
// library rather than hand-rolled here.
type StreamBus struct {
	client    redis.UniversalClient
	logger    watermill.LoggerAdapter
	publisher *redisstream.Publisher
}

// NewStreamBus wraps client as a StreamBus, publishing through a
// watermill-redisstream Publisher configured with logger.
func NewStreamBus(client redis.UniversalClient, logger watermill.LoggerAdapter) (*StreamBus, error) {
	publisher, err := redisstream.NewPublisher(redisstream.PublisherConfig{Client: client}, logger)
	if err != nil {
		return nil, err
	}

	return &StreamBus{
		client:    client,
		logger:    logger,
		publisher: publisher,
	}, nil
}

// Fire appends event to stream and returns immediately (non-blocking).
func (b *StreamBus) Fire(ctx context.Context, stream string, event *Event) error {
	payload, err := MarshalEvent(event)
	if err != nil {
		return err
	}

	msg := message.NewMessage(watermill.NewUUID(), payload)
	msg.SetContext(ctx)
	return b.publisher.Publish(stream, msg)
}

// Consume runs a blocking read loop for a single consumer within group,
// invoking handler for each event and acknowledging it on success. It
// returns when ctx is canceled.
func (b *StreamBus) Consume(ctx context.Context, stream, group, consumer string, handler func(context.Context, *Event) error) error {
	subscriber, err := redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        b.client,
		ConsumerGroup: group,
		Consumer:      consumer,
	}, b.logger)
	if err != nil {
		return err
	}
	defer func() { _ = subscriber.Close() }()

	messages, err := subscriber.Subscribe(ctx, stream)
	if err != nil {
		return err
	}

	for msg := range messages {
		var event Event
		if err := UnmarshalEvent(msg.Payload, &event); err != nil {
			msg.Nack()
			continue
		}

		if err := handler(ctx, &event); err != nil {
			msg.Nack()
			continue
		}

		msg.Ack()
	}

	return ctx.Err()
}
