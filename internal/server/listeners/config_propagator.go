package listeners

import (
	"context"
	"log/slog"

	"Metarr/internal/shared/appconfig"
	"Metarr/internal/shared/correlation"
	"Metarr/internal/shared/scanmodel"
)

// liveConfigSetter swaps the process-wide live config singleton.
// liveConfigSetterFunc adapts the package-level appconfig.Set to this
// interface, so a test can substitute a spy instead of mutating real
// process-global state.
type liveConfigSetter interface {
	Set(cfg *appconfig.Config)
}

type liveConfigSetterFunc func(*appconfig.Config)

func (f liveConfigSetterFunc) Set(cfg *appconfig.Config) { f(cfg) }

// sidecarRegistrySetter compiles a sidecar type table and, on success,
// activates it as the process-wide active registry. sidecarRegistryAdapter
// is the production adapter over scanmodel's package-level functions.
type sidecarRegistrySetter interface {
	Compile(defs []*appconfig.SidecarTypeDefinition) error
}

type sidecarRegistryAdapter struct{}

func (sidecarRegistryAdapter) Compile(defs []*appconfig.SidecarTypeDefinition) error {
	registry, err := scanmodel.NewSidecarRegistry(defs)
	if err != nil {
		return err
	}
	scanmodel.SetSidecarRegistry(registry)
	return nil
}

// logLevelSetter is the propagator's log-verbosity dependency, satisfied
// directly by *logging.Shipper.
type logLevelSetter interface {
	SetLevel(level slog.Level)
}

// agentPublisher republishes every agent's redacted config projection,
// satisfied directly by *agentregistry.Registry.
type agentPublisher interface {
	PublishAll(ctx context.Context, config *appconfig.Config) error
}

// ConfigPropagator is what happens to a config document once the config
// store has persisted it: swap the live config singleton so the rest of the
// process sees it immediately, apply the server log level, recompile the
// sidecar classification registry, and republish each agent's own view. It
// always takes an already-decoded *appconfig.Config, so a test never needs a
// JSON round-trip to exercise it.
//
// appconfigstore.Store.MutateSync calls PropagateInProcess after its own
// Mongo write; that is the propagator's one entry point.
type ConfigPropagator struct {
	liveConfig      liveConfigSetter
	sidecarRegistry sidecarRegistrySetter
	agents          agentPublisher
	logShipper      logLevelSetter
	logger          *slog.Logger
}

// NewConfigPropagator wires the production propagator: agent projections from
// agents, the live-config singleton and the sidecar registry from their
// appconfig / scanmodel package-level functions.
func NewConfigPropagator(
	agents agentPublisher,
	logShipper logLevelSetter,
	logger *slog.Logger,
) *ConfigPropagator {
	return newConfigPropagator(liveConfigSetterFunc(appconfig.Set), sidecarRegistryAdapter{}, agents, logShipper, logger)
}

// newConfigPropagator is the all-dependencies constructor, kept so a test can
// substitute a spy for the live-config singleton and the sidecar registry.
func newConfigPropagator(
	liveConfig liveConfigSetter,
	sidecarRegistry sidecarRegistrySetter,
	agents agentPublisher,
	logShipper logLevelSetter,
	logger *slog.Logger,
) *ConfigPropagator {
	return &ConfigPropagator{
		liveConfig:      liveConfig,
		sidecarRegistry: sidecarRegistry,
		agents:          agents,
		logShipper:      logShipper,
		logger:          logger,
	}
}

// PropagateInProcess makes an already-persisted cfg live: the caller (the
// config store's synchronous write path) owns the Mongo write, so this only
// swaps the singleton, sets the log level, recompiles the sidecar registry
// and republishes agent projections. Every failure is logged, never returned
// — the durable write has already succeeded, and agents re-read on their own
// timer regardless.
func (p *ConfigPropagator) PropagateInProcess(ctx context.Context, cfg *appconfig.Config) error {
	correlationID := correlation.FromContext(ctx)
	cfg = appconfig.Normalize(cfg)

	p.liveConfig.Set(cfg)

	// The server's own verbosity, applied live — the same SetLevel an
	// agent's ConfigStore calls, just driven by the local config swap
	// above instead of a Redis-delivered projection.
	level := slog.LevelInfo
	if cfg.Logging.ServerLevel == appconfig.LogLevelDebug {
		level = slog.LevelDebug
	}
	p.logShipper.SetLevel(level)

	// A table that fails to compile leaves the previous registry in place:
	// the scanner must never be left without one, and the previous table is
	// a far better answer than nothing.
	if err := p.sidecarRegistry.Compile(cfg.DirectoryScanner.SidecarTypes); err != nil {
		p.logger.Error("updated sidecar type table is invalid; keeping the previous one",
			"correlation_id", correlationID, "error", err)
	}

	if err := p.agents.PublishAll(ctx, cfg); err != nil {
		p.logger.Error("failed to republish agent configuration",
			"correlation_id", correlationID, "error", err)
	}
	return nil
}
