package runtime

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/logging"
	"Metarr/internal/shared/scanmodel"
)

// configPollInterval is the safety net behind the change notification.
//
// Pub/Sub delivers to whoever is connected at that instant, so a notification
// sent while the agent was reconnecting is gone. Re-reading on a slow timer
// turns that from "wrong configuration until someone notices" into "wrong
// configuration for up to a minute".
const configPollInterval = time.Minute

// ConfigStore holds the agent's current configuration projection and keeps it
// in step with the copy the server publishes to Redis.
//
// A nil projection is a normal state, not an error: an agent that has
// connected but has not been configured yet has nothing to do, and says so in
// the UI rather than failing.
type ConfigStore struct {
	client  redis.UniversalClient
	logger  *slog.Logger
	slug    string
	shipper *logging.Shipper
	current atomic.Pointer[agentproto.AgentConfigProjection]
}

// NewConfigStore returns an empty store for the given agent slug. shipper is
// this agent's own logging.Shipper — Refresh calls its SetLevel whenever a
// projection arrives, so the System > Logging screen's per-agent level
// toggle takes effect without a restart.
func NewConfigStore(client redis.UniversalClient, logger *slog.Logger, slug string, shipper *logging.Shipper) *ConfigStore {
	return &ConfigStore{client: client, logger: logger, slug: slug, shipper: shipper}
}

// Current returns the latest projection, or nil when the agent has not been
// configured yet.
func (s *ConfigStore) Current() *agentproto.AgentConfigProjection {
	return s.current.Load()
}

// Refresh re-reads the projection from Redis and installs it.
func (s *ConfigStore) Refresh(ctx context.Context) error {
	raw, err := s.client.Get(ctx, agentproto.ConfigKey(s.slug)).Result()
	if err == redis.Nil {
		// Not configured yet. Keep whatever we had rather than dropping a
		// working configuration because one read came back empty.
		if s.current.Load() == nil {
			s.logger.Info("no configuration published for this agent yet; waiting")
		}
		return nil
	}
	if err != nil {
		return err
	}

	projection := &agentproto.AgentConfigProjection{}
	if err := agentproto.UnmarshalStored([]byte(raw), projection); err != nil {
		return err
	}

	previous := s.current.Swap(projection)
	s.applySidecarTypes(projection)
	s.applyLogLevel(projection)

	if previous == nil {
		s.logger.Info("configuration received",
			"directories", len(projection.Directories),
			"parallel_count", projection.ParallelCount,
			"updated_at", projection.UpdatedAt.AsTime(),
		)
	} else if !previous.UpdatedAt.AsTime().Equal(projection.UpdatedAt.AsTime()) {
		s.logger.Info("configuration updated",
			"directories", len(projection.Directories),
			"updated_at", projection.UpdatedAt.AsTime(),
		)
	}
	return nil
}

// applySidecarTypes recompiles the sidecar classification table.
//
// The scanner reads this through a package global, so it has to be installed
// rather than passed. A table that fails to compile leaves the previous one
// in place: classifying with the last known-good rules beats refusing to scan.
func (s *ConfigStore) applySidecarTypes(projection *agentproto.AgentConfigProjection) {
	if len(projection.SidecarTypes) == 0 {
		return
	}

	registry, err := scanmodel.NewSidecarRegistry(projection.SidecarTypes)
	if err != nil {
		s.logger.Error("published sidecar table did not compile; keeping the previous one", "error", err)
		return
	}
	scanmodel.SetSidecarRegistry(registry)
}

// applyLogLevel sets this agent's live log level from the published
// projection. An unrecognized value is treated as info rather than rejected
// outright — an agent must never end up with no threshold at all just
// because a future level name it doesn't know about arrived.
func (s *ConfigStore) applyLogLevel(projection *agentproto.AgentConfigProjection) {
	if s.shipper == nil {
		return
	}

	level := slog.LevelInfo
	if projection.LogLevel == appconfig.LogLevelDebug {
		level = slog.LevelDebug
	}
	s.shipper.SetLevel(level)
}

// Register wires the server's change notification onto router: when the server
// publishes to this agent's AgentConfigChangedChannel, the handler re-reads the
// projection from Redis and installs it. The router owns the subscription loop
// and its shutdown, matching the NFO responder and the server's log listeners.
//
// The belt-and-braces periodic re-read is deliberately not entangled with this
// wake-up — it runs on its own goroutine in RefreshPeriodically so a missed
// notification is still bounded even if this subscription is briefly down.
func (s *ConfigStore) Register(router *eventbus.PubSubRouter) {
	channel := eventbus.AgentConfigChangedChannel(s.slug)
	router.Handle(channel, func(ctx context.Context, _ []byte) {
		if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("failed to re-read configuration after a change", "error", err)
		}
	})
	s.logger.Info("config-changed watch registered", "channel", channel)
}

// RefreshPeriodically re-reads the projection on a slow timer until ctx is
// cancelled. It is the safety net behind the change notification wired by
// Register: Pub/Sub delivers only to whoever is connected at that instant, so a
// notification sent while this agent was reconnecting is gone, and a periodic
// re-read bounds how long that can leave the agent on stale configuration. It
// reads once on entry so a freshly started agent does not wait a full interval.
func (s *ConfigStore) RefreshPeriodically(ctx context.Context) {
	if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
		s.logger.Warn("failed to read configuration", "error", err)
	}

	ticker := time.NewTicker(configPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("failed to re-read configuration", "error", err)
			}
		}
	}
}
