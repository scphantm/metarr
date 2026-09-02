package eventbus

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
)

// PubSubTransport is the substitutable half of the Bus's Pub/Sub side
// (docs/design/eventbus-bus-interface.md §4 amendment). Production always
// runs on Redis: neither binary sets Config.PubSub, so New defaults it to a
// redisPubSub over the shared client, and the SUBSCRIBE-ack-before-publish
// and correlation-scoped-reply logic stays on the Bus side of this seam
// exactly as before. The seam exists so the whole Bus — not just its
// durable-stream half — is exercisable with no Redis, the same way
// ChannelStreamTransport does it for streams, and so the ADR-0008
// conformance harness can drive the Pub/Sub contract without a broker.
type PubSubTransport interface {
	// Subscribe opens a subscription to one channel. The returned
	// subscription is not guaranteed live until Receive returns.
	Subscribe(ctx context.Context, channel string) (PubSubSubscription, error)
	// Publish sends one payload to a channel, fire-and-forget. A channel
	// with no subscribers drops it, matching Redis Pub/Sub.
	Publish(ctx context.Context, channel string, payload []byte) error
}

// PubSubSubscription is one open subscription. It mirrors the slice of
// *redis.PubSub the Bus actually uses — the SUBSCRIBE ack (Receive), the
// message stream (Channel), and teardown (Close) — so the Redis adapter is a
// near-passthrough and the fake is small.
type PubSubSubscription interface {
	// Receive blocks until the SUBSCRIBE is acknowledged or ctx is done.
	// The Bus calls it once, before publishing, so a request cannot be
	// answered before the reply channel is listening.
	Receive(ctx context.Context) error
	// Channel delivers message payloads until Close, or the subscription's
	// context ends. It is closed when the subscription is torn down.
	Channel() <-chan []byte
	// Close ends the subscription and closes the Channel.
	Close() error
}

// --- Redis adapter -------------------------------------------------------

// redisPubSub is the production adapter: a thin wrapper over the shared
// go-redis client. It owns nothing — Close on a subscription closes only that
// subscription, never the client.
type redisPubSub struct {
	client redis.UniversalClient
}

// RedisPubSub is the durable Pub/Sub transport backed by the shared Redis
// client. New uses it as the default when Config.PubSub is unset, so call
// sites never name it; it is exported only for symmetry with
// RedisStreamTransport.
func RedisPubSub(client redis.UniversalClient) PubSubTransport {
	return redisPubSub{client: client}
}

func (r redisPubSub) Subscribe(ctx context.Context, channel string) (PubSubSubscription, error) {
	return &redisPubSubSubscription{ps: r.client.Subscribe(ctx, channel)}, nil
}

func (r redisPubSub) Publish(ctx context.Context, channel string, payload []byte) error {
	return r.client.Publish(ctx, channel, payload).Err()
}

type redisPubSubSubscription struct {
	ps   *redis.PubSub
	once sync.Once
	out  chan []byte
}

func (s *redisPubSubSubscription) Receive(ctx context.Context) error {
	_, err := s.ps.Receive(ctx)
	return err
}

// Channel adapts go-redis' *redis.Message stream to the []byte view the Bus
// wants. The forward goroutine exits when ps.Close makes the source range
// end; a slow consumer drops payloads (Pub/Sub is at-most-once) rather than
// wedging the goroutine.
func (s *redisPubSubSubscription) Channel() <-chan []byte {
	s.once.Do(func() {
		s.out = make(chan []byte, 128)
		go func() {
			defer close(s.out)
			for msg := range s.ps.Channel() {
				select {
				case s.out <- []byte(msg.Payload):
				default:
				}
			}
		}()
	})
	return s.out
}

func (s *redisPubSubSubscription) Close() error { return s.ps.Close() }

// --- In-memory adapter -------------------------------------------------

// inMemoryPubSub is the test adapter: an in-process broker with the two
// properties the Bus depends on — a subscription is live the instant
// Subscribe returns (so Receive is a no-op and the request/reply ordering
// holds), and Publish fans one payload out to every subscriber on the
// channel.
type inMemoryPubSub struct {
	mu   sync.Mutex
	subs map[string]map[*inMemorySubscription]struct{}
}

// InMemoryPubSub is the Pub/Sub transport for tests: no Redis, no broker.
// Pass it as Config.PubSub the way ChannelStreamTransport is passed as
// Config.Streams. One instance is one broker — every subscription and
// publish made through the Bus it is wired into shares it.
func InMemoryPubSub() PubSubTransport {
	return &inMemoryPubSub{subs: map[string]map[*inMemorySubscription]struct{}{}}
}

func (b *inMemoryPubSub) Subscribe(_ context.Context, channel string) (PubSubSubscription, error) {
	sub := &inMemorySubscription{broker: b, channel: channel, ch: make(chan []byte, 256)}
	b.mu.Lock()
	if b.subs[channel] == nil {
		b.subs[channel] = map[*inMemorySubscription]struct{}{}
	}
	b.subs[channel][sub] = struct{}{}
	b.mu.Unlock()
	return sub, nil
}

// Publish holds the broker lock across the fan-out so a concurrent Close
// cannot close a subscription's channel mid-send. Delivery is non-blocking:
// a full subscriber buffer drops the payload, as Redis Pub/Sub does.
func (b *inMemoryPubSub) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for sub := range b.subs[channel] {
		select {
		case sub.ch <- append([]byte(nil), payload...):
		default:
		}
	}
	return nil
}

func (b *inMemoryPubSub) remove(sub *inMemorySubscription) {
	b.mu.Lock()
	if subs := b.subs[sub.channel]; subs != nil {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.subs, sub.channel)
		}
	}
	b.mu.Unlock()
}

type inMemorySubscription struct {
	broker  *inMemoryPubSub
	channel string
	ch      chan []byte
	once    sync.Once
}

// Receive is a no-op: Subscribe registered this subscription synchronously,
// so it is already live.
func (s *inMemorySubscription) Receive(context.Context) error { return nil }

func (s *inMemorySubscription) Channel() <-chan []byte { return s.ch }

func (s *inMemorySubscription) Close() error {
	s.once.Do(func() {
		s.broker.remove(s)
		close(s.ch)
	})
	return nil
}
