package agentregistry

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/eventbus"
)

// Registry is the server's view of the agent fleet: who is currently alive,
// and what each of them has been told.
type Registry struct {
	client redis.UniversalClient
	bus    *eventbus.Bus
	logger *slog.Logger
}

// New returns a Registry backed by client. bus carries the best-effort
// config-changed notification to each agent.
func New(client redis.UniversalClient, bus *eventbus.Bus, logger *slog.Logger) *Registry {
	return &Registry{client: client, bus: bus, logger: logger}
}

// List returns every agent the server knows about as a metarr.v1.Agent: those
// configured in config, those currently present in Redis, and the union of the
// two — the operator fields from config, the output-only presence fields
// (configured, online, identity, telemetry, reported_at) from Redis.
//
// Both halves matter. A configured agent that has gone away must still appear,
// or a machine going offline would look like a machine that was never set up.
// An unconfigured agent that has appeared must also show, because that is the
// only way anyone learns it is there. LogLevel is defaulted the same way
// BuildProjection defaults it, so the Logging screen's toggle always has a
// real value.
func (r *Registry) List(ctx context.Context, config *appconfig.Config) ([]*metarrv1.Agent, error) {
	presence, err := r.presence(ctx)
	if err != nil {
		return nil, err
	}

	agents := make(map[string]*metarrv1.Agent, len(presence)+len(config.Agents))

	for _, agent := range config.Agents {
		logLevel := agent.LogLevel
		if logLevel == "" {
			logLevel = appconfig.LogLevelInfo
		}
		agents[agent.Slug] = &metarrv1.Agent{
			Slug:        agent.Slug,
			DisplayName: agent.DisplayName,
			Configured:  true,
			Mappings:    cloneMappings(agent.Mappings),
			LogLevel:    logLevel,
		}
	}

	for slug, reported := range presence {
		agent, known := agents[slug]
		if !known {
			agent = &metarrv1.Agent{Slug: slug, Mappings: []*appconfig.AgentDirectoryMapping{}, LogLevel: appconfig.LogLevelInfo}
			agents[slug] = agent
		}

		agent.Online = true
		agent.Identity = reported.Identity
		agent.Telemetry = reported.Telemetry
		agent.ReportedAt = reported.ReportedAt
	}

	list := make([]*metarrv1.Agent, 0, len(agents))
	for _, agent := range agents {
		list = append(list, agent)
	}
	// Stable order so the UI does not reshuffle its cards on every poll.
	sort.Slice(list, func(i, j int) bool { return list[i].Slug < list[j].Slug })

	return list, nil
}

func cloneMappings(mappings []*appconfig.AgentDirectoryMapping) []*appconfig.AgentDirectoryMapping {
	out := make([]*appconfig.AgentDirectoryMapping, 0, len(mappings))
	for _, mapping := range mappings {
		out = append(out, proto.Clone(mapping).(*appconfig.AgentDirectoryMapping))
	}
	return out
}

// presence reads every live agent's presence record.
func (r *Registry) presence(ctx context.Context) (map[string]*agentproto.AgentPresence, error) {
	found := map[string]*agentproto.AgentPresence{}

	// SCAN rather than KEYS: this runs on a poll behind a live UI, and KEYS
	// blocks the whole server for the duration of the sweep.
	iterator := r.client.Scan(ctx, 0, agentproto.PresenceKeyPattern, 100).Iterator()
	for iterator.Next(ctx) {
		key := iterator.Val()

		raw, err := r.client.Get(ctx, key).Result()
		if err == redis.Nil {
			// Expired between the scan and the read. That is the agent going
			// offline, which is exactly what it looks like from here.
			continue
		}
		if err != nil {
			return nil, err
		}

		presence := &agentproto.AgentPresence{}
		if err := agentproto.UnmarshalStored([]byte(raw), presence); err != nil {
			r.logger.Warn("skipping unreadable agent presence record", "key", key, "error", err)
			continue
		}

		slug := agentproto.SlugFromPresenceKey(key)
		if slug == "" {
			continue
		}
		found[slug] = presence
	}
	if err := iterator.Err(); err != nil {
		return nil, err
	}

	return found, nil
}

// PresentSlugs returns the slug of every agent with a live presence key,
// reusing the same SCAN List runs. It is the PresenceWatcher's read
// dependency; there is deliberately no point-in-time "is agent X online"
// check any more — a scan dispatch that used one had a time-of-check-to-
// time-of-use gap against the agent consuming its durable command stream
// (docs/adr/0006).
func (r *Registry) PresentSlugs(ctx context.Context) ([]string, error) {
	presence, err := r.presence(ctx)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(presence))
	for slug := range presence {
		slugs = append(slugs, slug)
	}
	return slugs, nil
}

// PublishAll rewrites the configuration projection for every configured agent
// and tells each of them to re-read it.
//
// Projections are written for configured agents only. An agent that has merely
// appeared gets nothing to read, which is what keeps it idle until someone
// decides what it is allowed to see.
func (r *Registry) PublishAll(ctx context.Context, config *appconfig.Config) error {
	updatedAt := time.Now().UTC()

	for _, agent := range config.Agents {
		if err := r.publish(ctx, config, agent.Slug, updatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) publish(ctx context.Context, config *appconfig.Config, slug string, updatedAt time.Time) error {
	projection := BuildProjection(config, slug, updatedAt)

	encoded, err := agentproto.MarshalStored(projection)
	if err != nil {
		return err
	}

	if err := r.client.Set(ctx, agentproto.ConfigKey(slug), encoded, 0).Err(); err != nil {
		return err
	}

	// Best effort: an agent that misses this re-reads on its own timer, so a
	// failed notification is a delay rather than a stale configuration.
	if err := r.bus.Notify(ctx, eventbus.AgentConfigChangedTopic(slug), []byte("changed")); err != nil {
		r.logger.Debug("could not notify agent of a configuration change", "agent", slug, "error", err)
	}
	return nil
}

// Forget removes an agent's published configuration. Used when an agent is
// deleted, so a machine that reconnects later does not resume scanning a
// library it is no longer meant to touch.
func (r *Registry) Forget(ctx context.Context, slug string) error {
	return r.client.Del(ctx, agentproto.ConfigKey(slug)).Err()
}
