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
// It now registers on the shared PubSubRouter like every other notification
// consumer; this proves the wake-up still re-reads the projection when the
// server publishes to the agent's AgentConfigChangedChannel.
func TestConfigStoreRegisterRefreshesOnChangeNotification(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	const slug = "nas-01"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewConfigStore(client, logger, slug, nil)

	router := eventbus.NewPubSubRouter(client, eventbus.AgentSource(slug), logger)
	store.Register(router)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = router.Run(ctx) }()
	select {
	case <-router.Running():
	case <-time.After(2 * time.Second):
		t.Fatal("router never signalled Running()")
	}

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

	if err := client.Publish(ctx, eventbus.AgentConfigChangedChannel(slug), "changed").Err(); err != nil {
		t.Fatalf("publish change notification: %v", err)
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
