package eventbus

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-redisstream/pkg/redisstream"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
	"github.com/redis/go-redis/v9"
)

// StreamTransport is the one real port the Bus is built on (docs/adr/0008,
// docs/design/eventbus-bus-interface.md §4): the durable-stream transport,
// with a production Redis Streams adapter and an in-memory gochannel adapter
// for tests. Everything else the Bus needs — envelope assembly, dispatch,
// the retry stack, the retention sweep — lives on the near side of this seam
// so it has one implementation regardless of adapter.
type StreamTransport interface {
	// Publisher returns the append side. The Bus builds it inside Run, once
	// the late-bound policy has been read, so the caller passes the per-entry
	// MAXLEN cap on every Publish rather than fixing it at construction.
	Publisher() (StreamPublisher, error)
	// Subscriber opens a durable-stream subscriber bound to one consumer
	// group and consumer identity. The delivered message.Payload is the
	// envelope JSON — the Redis adapter's unmarshaller strips the
	// {"payload": …} wrapper, the channel adapter passes it through.
	Subscriber(group, consumer string) (message.Subscriber, error)
}

// StreamPublisher appends one durable-stream entry carrying envelope as its
// payload. The adapter owns the entry framing (the minimal
// {"payload": <envelope JSON>} marshaller — docs/adr/0006 2026-09-01
// amendment) and the approximate MAXLEN cap; the Bus owns envelope assembly
// and protojson. Close never closes a shared Redis client.
type StreamPublisher interface {
	Publish(ctx context.Context, stream string, envelope []byte, approxMaxLen int64) error
	Close() error
}

// streamPayloadField is the single Redis Stream entry field every bus entry
// carries. No _watermill_message_uuid, no msgpack metadata blob — just the
// envelope JSON, so a participant in another language reads one field it
// already understands (docs/adr/0006 2026-09-01, docs/adr/0008).
const streamPayloadField = "payload"

// minimalStreamValues is the one place a bus stream entry's on-the-wire shape
// is written: exactly one field, the envelope JSON.
func minimalStreamValues(envelope []byte) map[string]any {
	return map[string]any{streamPayloadField: envelope}
}

// payloadFromStreamValues is the inverse: it recovers the envelope JSON from
// a decoded stream entry, rejecting an entry that does not carry the one
// field this bus writes.
func payloadFromStreamValues(values map[string]any) ([]byte, error) {
	raw, ok := values[streamPayloadField]
	if !ok || raw == nil {
		return nil, fmt.Errorf("eventbus: stream entry has no %q field", streamPayloadField)
	}
	switch payload := raw.(type) {
	case string:
		return []byte(payload), nil
	case []byte:
		return payload, nil
	default:
		return nil, fmt.Errorf("eventbus: stream entry %q field is %T, want string", streamPayloadField, raw)
	}
}

// minimalUnmarshaller decodes a Redis Stream entry written by
// minimalStreamValues back into a Watermill message whose Payload is the
// envelope JSON. The UUID is minted fresh: the bus does not carry
// Watermill's message id on the wire, and nothing downstream dispatches on
// it (the envelope's own correlation_id is the trace key).
type minimalUnmarshaller struct{}

func (minimalUnmarshaller) Unmarshal(values map[string]any) (*message.Message, error) {
	payload, err := payloadFromStreamValues(values)
	if err != nil {
		return nil, err
	}
	return message.NewMessage(watermill.NewUUID(), payload), nil
}

// --- Redis adapter ---------------------------------------------------------

// redisStreamTransport is the production adapter: watermill-redisstream
// subscribers for the consume side (keeping the library's consumer-group
// creation and XAUTOCLAIM reclaim) and a thin XADD wrapper for the append
// side (so the MAXLEN cap is per-entry and Close owns nothing).
type redisStreamTransport struct {
	client redis.UniversalClient
	logger watermill.LoggerAdapter
}

// RedisStreamTransport is the durable-stream transport backed by Redis
// Streams over the shared client. The client is shared and not owned —
// nothing here ever closes it.
func RedisStreamTransport(client redis.UniversalClient, logger watermill.LoggerAdapter) StreamTransport {
	return &redisStreamTransport{client: client, logger: logger}
}

func (t *redisStreamTransport) Publisher() (StreamPublisher, error) {
	return &redisStreamPublisher{client: t.client}, nil
}

func (t *redisStreamTransport) Subscriber(group, consumer string) (message.Subscriber, error) {
	return redisstream.NewSubscriber(redisstream.SubscriberConfig{
		Client:        t.client,
		ConsumerGroup: group,
		Consumer:      consumer,
		Unmarshaller:  minimalUnmarshaller{},
	}, t.logger)
}

// redisStreamPublisher appends one entry per call with the given approximate
// MAXLEN. It does not use redisstream.Publisher: that fixes MAXLEN at
// construction and its Close closes the shared client. A direct XADD keeps
// the cap late-bound and Close a genuine no-op.
type redisStreamPublisher struct {
	client redis.UniversalClient
}

func (p *redisStreamPublisher) Publish(ctx context.Context, stream string, envelope []byte, approxMaxLen int64) error {
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: minimalStreamValues(envelope),
		MaxLen: approxMaxLen,
		Approx: true,
	}).Err()
}

// Close is deliberately a no-op: this publisher owns nothing. The Redis
// client is shared and closed by whoever built it (see StreamBus.Close for
// the same reasoning).
func (p *redisStreamPublisher) Close() error { return nil }

// --- Channel adapter -----------------------------------------------------

// channelStreamTransport is the test adapter: one in-memory gochannel is both
// the publisher and every subscriber, so handler logic, the middleware stack
// and per-(topic, name) dispatch are exercised with no Redis. group and
// consumer are ignored — gochannel has no consumer groups.
type channelStreamTransport struct {
	channel *gochannel.GoChannel
}

// ChannelStreamTransport is the in-memory durable-stream transport for tests.
func ChannelStreamTransport() StreamTransport {
	return &channelStreamTransport{
		channel: gochannel.NewGoChannel(gochannel.Config{}, watermill.NopLogger{}),
	}
}

func (t *channelStreamTransport) Publisher() (StreamPublisher, error) {
	return &channelStreamPublisher{channel: t.channel}, nil
}

func (t *channelStreamTransport) Subscriber(_, _ string) (message.Subscriber, error) {
	return t.channel, nil
}

type channelStreamPublisher struct {
	channel *gochannel.GoChannel
}

func (p *channelStreamPublisher) Publish(ctx context.Context, stream string, envelope []byte, _ int64) error {
	msg := message.NewMessage(watermill.NewUUID(), envelope)
	msg.SetContext(ctx)
	return p.channel.Publish(stream, msg)
}

func (p *channelStreamPublisher) Close() error { return p.channel.Close() }
