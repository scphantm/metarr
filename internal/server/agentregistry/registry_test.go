package agentregistry

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// PublishAll writes each configured agent's projection and then tells the
// agent to re-read it. The notification now travels through bus.Notify on the
// agent's config-changed topic rather than a raw Redis publish; this proves a
// HandleNotify subscriber still wakes, and the projection key is still
// written.
func TestPublishAllWritesProjectionAndNotifiesThroughTheBus(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus, err := eventbus.New(eventbus.Config{
		Redis:   client,
		Source:  eventbus.SourceServer,
		Streams: eventbus.ChannelStreamTransport(),
		Policy:  eventbus.DefaultBusPolicy,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("eventbus.New: %v", err)
	}

	const slug = "nas-01"
	notified := make(chan struct{}, 1)
	if err := bus.HandleNotify(eventbus.AgentConfigChangedTopic(slug), func(context.Context, []byte) {
		select {
		case notified <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("HandleNotify: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- bus.Run(ctx) }()
	select {
	case <-bus.Ready():
	case err := <-runDone:
		t.Fatalf("bus stopped before ready: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("bus never became ready")
	}
	t.Cleanup(func() {
		cancel()
		<-runDone
		_ = bus.Close()
	})

	registry := New(client, bus, logger)
	if err := registry.PublishAll(ctx, configWithEverySecret()); err != nil {
		t.Fatalf("PublishAll: %v", err)
	}

	if err := client.Get(ctx, agentproto.ConfigKey(slug)).Err(); err != nil {
		t.Fatalf("projection key was not written: %v", err)
	}

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("agent was never notified of the configuration change")
	}
}
