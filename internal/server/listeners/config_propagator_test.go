package listeners

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"Metarr/internal/shared/appconfig"
)

type fakePersister struct {
	err       error
	upsertCfg *appconfig.Config
}

func (f *fakePersister) Upsert(_ context.Context, cfg *appconfig.Config) error {
	f.upsertCfg = cfg
	return f.err
}

type fakeLiveConfigSetter struct {
	cfg *appconfig.Config
}

func (f *fakeLiveConfigSetter) Set(cfg *appconfig.Config) { f.cfg = cfg }

type fakeSidecarRegistry struct {
	err     error
	compile []appconfig.SidecarTypeDefinition
	calls   int
}

func (f *fakeSidecarRegistry) Compile(defs []appconfig.SidecarTypeDefinition) error {
	f.calls++
	f.compile = defs
	return f.err
}

type fakeLogLevelSetter struct {
	level     slog.Level
	setCalled bool
}

func (f *fakeLogLevelSetter) SetLevel(level slog.Level) {
	f.level = level
	f.setCalled = true
}

type fakeAgentPublisher struct {
	err   error
	calls int
}

func (f *fakeAgentPublisher) PublishAll(_ context.Context, _ *appconfig.Config) error {
	f.calls++
	return f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestPropagator() (*configPropagator, *fakePersister, *fakeLiveConfigSetter, *fakeSidecarRegistry, *fakeAgentPublisher, *fakeLogLevelSetter) {
	persist := &fakePersister{}
	live := &fakeLiveConfigSetter{}
	sidecar := &fakeSidecarRegistry{}
	agents := &fakeAgentPublisher{}
	logLevel := &fakeLogLevelSetter{}
	return newConfigPropagator(persist, live, sidecar, agents, logLevel, discardLogger()), persist, live, sidecar, agents, logLevel
}

func TestApply_HappyPathInvokesEveryDependency(t *testing.T) {
	propagator, persist, live, sidecar, agents, logLevel := newTestPropagator()
	cfg := &appconfig.Config{Logging: appconfig.LoggingConfig{ServerLevel: appconfig.LogLevelDebug}}

	if err := propagator.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if persist.upsertCfg != cfg {
		t.Error("expected the config to be persisted")
	}
	if live.cfg != cfg {
		t.Error("expected the live config singleton to be swapped")
	}
	if sidecar.calls != 1 {
		t.Errorf("expected the sidecar registry to be compiled once, got %d calls", sidecar.calls)
	}
	if agents.calls != 1 {
		t.Errorf("expected agent projections to be republished once, got %d calls", agents.calls)
	}
	if !logLevel.setCalled || logLevel.level != slog.LevelDebug {
		t.Errorf("expected log level Debug to be applied, got called=%v level=%v", logLevel.setCalled, logLevel.level)
	}
}

func TestApply_PersistFailureIsAHardErrorAndStopsPropagation(t *testing.T) {
	propagator, _, live, sidecar, agents, _ := newTestPropagator()
	sentinel := errors.New("mongo unavailable")
	propagator.persist = &fakePersister{err: sentinel}

	err := propagator.Apply(context.Background(), &appconfig.Config{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the persist error back unchanged, got %v", err)
	}
	if live.cfg != nil {
		t.Error("expected the live config swap to be skipped after a persist failure")
	}
	if sidecar.calls != 0 {
		t.Error("expected the sidecar registry compile to be skipped after a persist failure")
	}
	if agents.calls != 0 {
		t.Error("expected agent republish to be skipped after a persist failure")
	}
}

func TestApply_SidecarRegistryFailureIsLoggedNotReturned(t *testing.T) {
	propagator, _, live, sidecar, agents, _ := newTestPropagator()
	sidecar.err = errors.New("invalid regex")

	if err := propagator.Apply(context.Background(), &appconfig.Config{}); err != nil {
		t.Fatalf("expected no error, sidecar registry failures are log-only, got %v", err)
	}
	if live.cfg == nil {
		t.Error("expected the live config swap to have already happened")
	}
	if agents.calls != 1 {
		t.Error("expected agent republish to still run after a sidecar registry failure")
	}
}

func TestApply_AgentPublishFailureIsLoggedNotReturned(t *testing.T) {
	propagator, _, _, sidecar, agents, _ := newTestPropagator()
	agents.err = errors.New("redis unavailable")

	if err := propagator.Apply(context.Background(), &appconfig.Config{}); err != nil {
		t.Fatalf("expected no error, agent publish failures are log-only, got %v", err)
	}
	if sidecar.calls != 1 {
		t.Error("expected the sidecar registry compile to have already run")
	}
}

func TestLiveConfigSetterFunc_AdaptsAPlainFunctionToTheInterface(t *testing.T) {
	var got *appconfig.Config
	var setter liveConfigSetter = liveConfigSetterFunc(func(cfg *appconfig.Config) { got = cfg })

	cfg := &appconfig.Config{}
	setter.Set(cfg)

	if got != cfg {
		t.Error("expected the wrapped function to receive the config")
	}
}
