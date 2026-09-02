package runtime

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/eventbus"
)

// The config-changed watch used to run its own hand-rolled Redis Pub/Sub loop.
// It now registers a notify handler on the shared Bus like every other
// notification consumer; this proves the wake-up still re-reads the projection
// when the server notifies the agent's AgentConfigChangedTopic.
func TestConfigStoreRegisterRefreshesOnChangeNotification(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	const slug = "nas-01"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewConfigStore(client, logger, slug, nil)

	bus, err := eventbus.New(eventbus.Config{
		Redis:   client,
		Source:  eventbus.AgentSource(slug),
		Streams: eventbus.ChannelStreamTransport(),
		Policy:  eventbus.DefaultBusPolicy,
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("eventbus.New: %v", err)
	}
	if err := store.Register(bus); err != nil {
		t.Fatalf("store.Register: %v", err)
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

	// Nothing published yet: the store has no projection.
	if store.Current() != nil {
		t.Fatalf("Current() = %v before any config was published, want nil", store.Current())
	}

	projection := &agentproto.AgentConfigProjection{
		ParallelCount: 3,
		UpdatedAt:     timestamppb.New(time.Now().UTC()),
	}
	stored, err := agentproto.MarshalStored(projection)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	if err := client.Set(ctx, agentproto.ConfigKey(slug), stored, 0).Err(); err != nil {
		t.Fatalf("seed config key: %v", err)
	}

	if err := bus.Notify(ctx, eventbus.AgentConfigChangedTopic(slug), []byte("changed")); err != nil {
		t.Fatalf("notify change: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		if current := store.Current(); current != nil {
			if current.ParallelCount != 3 {
				t.Fatalf("ParallelCount = %d, want 3", current.ParallelCount)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("store never picked up the published projection after the change notification")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
