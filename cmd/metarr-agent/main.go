// Command metarr-agent runs the Metarr filesystem agent.
//
// The agent is deployed next to the media it scans — typically on the NAS
// itself — and connects to nothing but Redis. It holds no database credentials
// and never opens a database connection; every instruction it receives and
// every result it produces travels over the event bus.
//
// Locally it is configured with only two things: how to reach Redis, and what
// this agent is called. Everything else — which libraries it can see, where
// they live on this machine, how the scanner is tuned — is published to it by
// the server. An agent that starts before anyone has configured it is a normal
// state: it reports itself as present and waits.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"Metarr/internal/agent/hostinfo"
	"Metarr/internal/agent/runtime"
	"Metarr/internal/shared/agentproto"
	"Metarr/internal/shared/config"
	"Metarr/internal/shared/eventbus"
	"Metarr/internal/shared/logging"
	"Metarr/internal/shared/redisclient"
	"Metarr/internal/shared/version"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAgent()
	if err != nil {
		return err
	}

	if err := agentproto.ValidateSlug(cfg.Slug); err != nil {
		return err
	}

	// The source tag every log record carries — "metarr-agent-<slug>" — is
	// what lets logs from many agents share one OpenObserve stream and still
	// be filtered down to just this machine.
	logger, logShipper := logging.New("metarr-agent-" + cfg.Slug)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
	defer cancelConnect()

	redisClient, err := redisclient.New(connectCtx, cfg.RedisURI)
	if err != nil {
		return err
	}
	defer func() { _ = redisClient.Close() }()

	logShipper.Attach(redisClient)

	identity := hostinfo.Identity(cfg.Slug, uuid.NewString(), version.Raw, time.Now().UTC())

	presence := runtime.NewPresence(redisClient, logger, identity)
	if err := presence.Claim(connectCtx); err != nil {
		// A duplicate slug is a configuration mistake that only gets worse if
		// the process carries on, so it is fatal and says exactly what is wrong.
		if errors.Is(err, runtime.ErrSlugInUse) {
			logger.Error("cannot start: another agent is already running with this slug",
				"slug", cfg.Slug,
				"hint", "give this agent its own slug, or stop the other one",
			)
		}
		return err
	}

	logger.Info("agent starting",
		"slug", identity.Slug,
		"instance_id", identity.InstanceId,
		"hostname", identity.Hostname,
		"ip", identity.Ip,
		"uid", identity.Uid,
		"user", identity.Username,
		"platform", identity.Os+"/"+identity.Arch,
		"version", version.Raw,
	)

	// The agent has no live config to read (operator tuning of the event_bus
	// section does not reach agents, per ADR-0006), so it assembles the one
	// BusPolicy from the built-in defaults and passes the sub-slices on.
	busPolicy := eventbus.DefaultBusPolicy()
	streamBus, err := eventbus.NewStreamBus(redisClient, busPolicy.Retention, eventbus.NewSlogAdapter(logger))
	if err != nil {
		return err
	}
	pubsubBus := eventbus.NewPubSubBus(redisClient)

	configStore := runtime.NewConfigStore(redisClient, logger, cfg.Slug, logShipper)
	if err := configStore.Refresh(connectCtx); err != nil {
		// Not fatal: an unreachable config key is a reason to keep heartbeating
		// and retry, not a reason to exit. The agent is visible in the UI either
		// way, which is what lets someone see that it needs attention.
		logger.Warn("could not read configuration at startup; will retry", "error", err)
	}

	// One Watermill Router per process consumes every durable stream this
	// agent reads, with the Recoverer/drop-after-retry/Retry middleware
	// stack; a command that errors past the retry cap is logged at error
	// level and acked rather than redelivered forever (docs/adr/0006). The
	// agent enforces dry-run and reports business failures as result events
	// itself, so the scan handler only ever returns an error for a message it
	// could not process at all.
	eventRouter, err := eventbus.NewRedisRouter(redisClient, busPolicy.Retry, eventbus.NewSlogAdapter(logger))
	if err != nil {
		return err
	}

	scanner := runtime.NewScanner(streamBus, configStore, logger, cfg.Slug)
	if err := scanner.Register(eventRouter); err != nil {
		return err
	}
	responder := runtime.NewResponder(pubsubBus, configStore, logger, cfg.Slug)

	// Every loop is tracked, so shutdown waits for a scan in flight rather than
	// killing it half way through and leaving the server holding a partial
	// library with no completion to sweep it.
	var workers sync.WaitGroup
	start := func(name string, fn func()) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			fn()
			logger.Debug("worker stopped", "worker", name)
		}()
	}

	start("presence", func() { presence.Run(ctx) })
	start("config", func() { configStore.Watch(ctx) })
	start("responder", func() { responder.Run(ctx) })
	start("router", func() {
		if err := eventRouter.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("event router stopped unexpectedly", "error", err)
		}
	})

	<-ctx.Done()
	logger.Info("agent shutting down")

	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		logger.Warn("workers did not stop in time; exiting anyway")
	}

	logger.Info("agent stopped")
	return nil
}
