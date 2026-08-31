package agentregistry

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/appconfig"
)

// Registry is the server's view of the agent fleet: who is currently alive,
// and what each of them has been told.
type Registry struct {
	client redis.UniversalClient
	logger *slog.Logger
}

// New returns a Registry backed by client.
func New(client redis.UniversalClient, logger *slog.Logger) *Registry {
	return &Registry{client: client, logger: logger}
}

// AgentView is one agent as the UI sees it: what the operator configured, what
// the agent itself reports, and whether it is currently there. MappingView is
// one of its library mappings, showing both machines' names for it. Both are
// aliases to their generated metarr.v1 messages — proto is the single
// definition for a model the UI reads. See docs/adr/0005.
//
// On the message, Online is presence rather than health (true while the agent
// still refreshes its key); Configured separates an agent that has announced
// itself from one someone has set up; LogLevel is defaulted the same way
// BuildProjection defaults it, so the Logging screen's toggle always has a
// real value.
type (
	AgentView   = metarrv1.AgentView
	MappingView = metarrv1.AgentMappingView
)

// List returns every agent the server knows about: those configured in config,
// those currently present in Redis, and the union of the two.
//
// Both halves matter. A configured agent that has gone away must still appear,
// or a machine going offline would look like a machine that was never set up.
// An unconfigured agent that has appeared must also show, because that is the
// only way anyone learns it is there.
func (r *Registry) List(ctx context.Context, config *appconfig.Config) ([]*AgentView, error) {
	presence, err := r.presence(ctx)
	if err != nil {
		return nil, err
	}

	views := make(map[string]*AgentView, len(presence)+len(config.Agents))

	for _, agent := range config.Agents {
		logLevel := agent.LogLevel
		if logLevel == "" {
			logLevel = appconfig.LogLevelInfo
		}
		views[agent.Slug] = &AgentView{
			Slug:        agent.Slug,
			DisplayName: agent.DisplayName,
			Configured:  true,
			Mappings:    mappingViews(config, agent),
			LogLevel:    logLevel,
		}
	}

	for slug, reported := range presence {
		view, known := views[slug]
		if !known {
			view = &AgentView{Slug: slug, Mappings: []*MappingView{}, LogLevel: appconfig.LogLevelInfo}
			views[slug] = view
		}

		view.Online = true
		view.Identity = reported.Identity
		view.Telemetry = reported.Telemetry
		view.ReportedAt = reported.ReportedAt
	}

	list := make([]*AgentView, 0, len(views))
	for _, view := range views {
		list = append(list, view)
	}
	// Stable order so the UI does not reshuffle its cards on every poll.
	sort.Slice(list, func(i, j int) bool { return list[i].Slug < list[j].Slug })

	return list, nil
}

func mappingViews(config *appconfig.Config, agent *appconfig.AgentConfig) []*MappingView {
	mappings := make([]*MappingView, 0, len(agent.Mappings))
	for _, mapping := range agent.Mappings {
		view := &MappingView{
			ScannerSlug: mapping.ScannerSlug,
			AgentPath:   mapping.AgentPath,
		}
		if index := appconfig.FindScanDirectoryIndex(config.DirectoryScanner, mapping.ScannerSlug); index >= 0 {
			view.ServerPath = config.DirectoryScanner.ScanDirectories[index].Directory
			view.ScanType = config.DirectoryScanner.ScanDirectories[index].ScanType
		}
		mappings = append(mappings, view)
	}
	return mappings
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

// IsOnline reports whether one agent is currently present.
func (r *Registry) IsOnline(ctx context.Context, slug string) (bool, error) {
	count, err := r.client.Exists(ctx, agentproto.PresenceKey(slug)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
	if err := r.client.Publish(ctx, agentproto.ConfigChangedChannel(slug), "changed").Err(); err != nil {
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
