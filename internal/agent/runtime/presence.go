// Package runtime is the agent's moving parts: claiming its identity,
// reporting that it is alive, keeping its configuration current, and running
// the work the server sends it.
package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/agent/hostinfo"
	"Metarr/internal/shared/agentproto"
)

// ErrSlugInUse is returned when another live agent already holds this slug.
//
// This is fatal on purpose. Two processes sharing a slug would each consume
// half the work sent to it and overwrite each other's presence, producing a
// system that looks healthy and behaves erratically — far worse than refusing
// to start.
var ErrSlugInUse = fmt.Errorf("agent slug is already claimed by another running agent")

// Presence claims the agent's slug and keeps its liveness key refreshed.
type Presence struct {
	client   redis.UniversalClient
	logger   *slog.Logger
	identity *agentproto.AgentIdentity
	metrics  *hostinfo.Collector
}

// NewPresence returns a Presence for identity.
func NewPresence(client redis.UniversalClient, logger *slog.Logger, identity *agentproto.AgentIdentity) *Presence {
	return &Presence{
		client:   client,
		logger:   logger,
		identity: identity,
		metrics:  hostinfo.NewCollector(),
	}
}

// Claim takes the slug lock, returning ErrSlugInUse if another agent holds it.
//
// The lock carries this instance's id, which is what makes a restart work: an
// agent that crashed still holds its lock until the TTL expires, and when it
// comes back it finds its own id there and takes it over rather than waiting
// out its own corpse.
func (p *Presence) Claim(ctx context.Context) error {
	key := agentproto.LockKey(p.identity.Slug)

	acquired, err := p.client.SetNX(ctx, key, p.identity.InstanceId, agentproto.PresenceTTL).Result()
	if err != nil {
		return fmt.Errorf("claiming agent slug: %w", err)
	}
	if acquired {
		return nil
	}

	holder, err := p.client.Get(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("reading agent slug lock: %w", err)
	}
	if holder == p.identity.InstanceId {
		return p.refreshLock(ctx)
	}

	return fmt.Errorf("%w (held by instance %s)", ErrSlugInUse, holder)
}

func (p *Presence) refreshLock(ctx context.Context) error {
	return p.client.Set(ctx, agentproto.LockKey(p.identity.Slug), p.identity.InstanceId, agentproto.PresenceTTL).Err()
}

// Report writes one presence record: identity plus a fresh telemetry sample.
func (p *Presence) Report(ctx context.Context) error {
	presence := &agentproto.AgentPresence{
		Identity:   p.identity,
		Telemetry:  p.metrics.Telemetry(ctx),
		ReportedAt: timestamppb.New(time.Now().UTC()),
	}

	encoded, err := agentproto.MarshalStored(presence)
	if err != nil {
		return err
	}

	if err := p.refreshLock(ctx); err != nil {
		return err
	}
	return p.client.Set(ctx, agentproto.PresenceKey(p.identity.Slug), encoded, agentproto.PresenceTTL).Err()
}

// Run reports presence until ctx is cancelled, then removes the keys so a
// cleanly stopped agent disappears immediately instead of lingering for the
// TTL. A killed agent has no such courtesy, which is exactly why the keys
// expire on their own as well.
func (p *Presence) Run(ctx context.Context) {
	ticker := time.NewTicker(agentproto.HeartbeatInterval)
	defer ticker.Stop()

	if err := p.Report(ctx); err != nil && ctx.Err() == nil {
		p.logger.Warn("failed to report presence", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			p.release()
			return
		case <-ticker.C:
			if err := p.Report(ctx); err != nil && ctx.Err() == nil {
				p.logger.Warn("failed to report presence", "error", err)
			}
		}
	}
}

func (p *Presence) release() {
	// A fresh context: the one that was cancelled cannot run these.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	slug := p.identity.Slug
	if err := p.client.Del(ctx, agentproto.PresenceKey(slug), agentproto.LockKey(slug)).Err(); err != nil {
		p.logger.Debug("could not clear presence keys on shutdown", "error", err)
	}
}
