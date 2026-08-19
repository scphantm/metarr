package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"Metarr/internal/shared/eventbus"
)

// selfReportInterval is how often the Shipper checks whether it has dropped
// or failed to ship anything since the last check, and if so says so. This
// is the package's own diagnostic channel — deliberately not routed back
// through the Handler it is reporting on, to keep a struggling pipeline from
// becoming its own noise source.
const selfReportInterval = 30 * time.Second

// Publisher is the one method Shipper needs from a Redis client. It is
// satisfied structurally by *redis.Client (and redis.UniversalClient), so
// callers pass the same client used everywhere else with no adapter — and
// tests can supply a two-line fake instead of standing up Redis.
type Publisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Shipper owns the buffer Handler enqueues into and the background goroutine
// that drains it. Nothing publishes to Redis until Attach is called; before
// that, and whenever Attach hasn't been given a reachable client, records
// simply accumulate up to the buffer's capacity and then drop — the same
// non-blocking behavior either way, just with nowhere for them to go yet.
type Shipper struct {
	buffer  chan []byte
	dropped *atomic.Int64
	failed  atomic.Int64
	level   *slog.LevelVar
}

func newShipper(bufferSize int, level *slog.LevelVar) *Shipper {
	shipper := &Shipper{
		buffer:  make(chan []byte, bufferSize),
		dropped: &atomic.Int64{},
		level:   level,
	}
	go shipper.reportLoop()
	return shipper
}

// SetLevel changes the minimum level this logger emits, live. Both the
// server's own level and every agent's are controlled through this single
// method — see the system_config_update listener and ConfigStore.Refresh.
func (s *Shipper) SetLevel(level slog.Level) {
	s.level.Set(level)
}

// Level returns the currently active minimum level.
func (s *Shipper) Level() slog.Level {
	return s.level.Level()
}

// Attach starts publishing buffered records to Redis on eventbus.LogChannel.
// Call it once, as soon as a Redis client exists — before that, Handle
// already behaves correctly (buffer-then-drop), so there is no ordering
// hazard in calling this after some logging has already happened.
func (s *Shipper) Attach(client Publisher) {
	go s.publishLoop(client)
}

func (s *Shipper) publishLoop(client Publisher) {
	ctx := context.Background()
	for encoded := range s.buffer {
		// A publish failure is deliberately not retried: retrying here would
		// mean holding records in memory waiting on a network call, which is
		// exactly the blocking behavior this package exists to avoid. A lost
		// record during a Redis or Fluent Bit hiccup is an acceptable cost for
		// an observability pipeline that must never slow down the app it is
		// observing.
		if err := client.Publish(ctx, eventbus.LogChannel, encoded).Err(); err != nil {
			s.failed.Add(1)
		}
	}
}

func (s *Shipper) reportLoop() {
	ticker := time.NewTicker(selfReportInterval)
	defer ticker.Stop()

	for range ticker.C {
		dropped := s.dropped.Swap(0)
		failed := s.failed.Swap(0)
		if dropped == 0 && failed == 0 {
			continue
		}
		// Straight to stderr, bypassing Handler entirely: this is a report
		// about the shipping pipeline itself, not a record for it to carry.
		fmt.Fprintf(os.Stderr,
			"logging: dropped %d records (buffer full), failed to publish %d in the last %s\n",
			dropped, failed, selfReportInterval,
		)
	}
}
