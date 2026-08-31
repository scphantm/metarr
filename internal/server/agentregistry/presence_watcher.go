package agentregistry

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"Metarr/internal/shared/agentproto"
)

// DefaultPresenceWatchInterval is how often the watcher re-scans the
// presence keys. Well under agentproto.PresenceTTL, so a key that stops
// being refreshed is noticed within a scan or two of it expiring.
const DefaultPresenceWatchInterval = 3 * time.Second

// PresenceLister is the watcher's one read dependency: the slugs of the
// agents with a live presence key right now. Satisfied by *Registry
// (PresentSlugs), which reuses the same SCAN Registry.List already runs, and
// by a fake in tests.
type PresenceLister interface {
	PresentSlugs(ctx context.Context) ([]string, error)
}

// PresenceWatcher is the one process-wide watcher for agents losing their
// presence key. When an agent that was present stops appearing in the scan
// for longer than agentproto.PresenceTTL, the watcher emits its slug once on
// every subscribed channel. In-flight work for that agent — a workflow node
// dispatched to it, once the engine exists — reacts to that instead of
// waiting out a long timeout (docs/adr/0006).
//
// The signal is edge-triggered on the presence key's existence: a key that
// is merely stale but still present is not offline, and produces nothing. An
// agent whose key vanishes, comes back, and vanishes again produces two
// signals. The TTL grace absorbs a single missed scan without a spurious
// signal.
type PresenceWatcher struct {
	lister   PresenceLister
	clock    func() time.Time
	ttlGrace time.Duration
	logger   *slog.Logger

	mu       sync.Mutex
	online   map[string]bool      // slugs believed present
	lastSeen map[string]time.Time // when each was last in a scan
	subs     []chan string
}

// NewPresenceWatcher builds a watcher over lister, reading the current time
// from clock. Both are injected so a test can drive presence and time by
// hand.
func NewPresenceWatcher(lister PresenceLister, clock func() time.Time, logger *slog.Logger) *PresenceWatcher {
	return &PresenceWatcher{
		lister:   lister,
		clock:    clock,
		ttlGrace: agentproto.PresenceTTL,
		logger:   logger,
		online:   map[string]bool{},
		lastSeen: map[string]time.Time{},
	}
}

// Subscribe returns a channel that receives the slug of each agent as it
// goes offline. The channel is buffered and a send that would block is
// dropped: a slow consumer misses a transition rather than stalling the
// watcher. Subscribe is expected to be called at wiring time, not
// per-request.
func (w *PresenceWatcher) Subscribe() <-chan string {
	w.mu.Lock()
	defer w.mu.Unlock()
	ch := make(chan string, 16)
	w.subs = append(w.subs, ch)
	return ch
}

// Run scans on an interval until ctx is cancelled.
func (w *PresenceWatcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := w.scanOnce(ctx); err != nil && ctx.Err() == nil {
			w.logger.Warn("presence watcher scan failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// scanOnce reconciles one presence scan against the watcher's belief and
// emits a signal for every agent that has now been absent past the grace
// window.
func (w *PresenceWatcher) scanOnce(ctx context.Context) error {
	slugs, err := w.lister.PresentSlugs(ctx)
	if err != nil {
		return err
	}

	now := w.clock()
	present := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		present[slug] = true
	}

	var wentOffline []string

	w.mu.Lock()
	for slug := range present {
		w.lastSeen[slug] = now
		if !w.online[slug] {
			w.online[slug] = true
		}
	}
	for slug, isOnline := range w.online {
		if !isOnline || present[slug] {
			continue
		}
		if now.Sub(w.lastSeen[slug]) < w.ttlGrace {
			// Absent for less than one TTL: treat as a missed scan, not a
			// departure, and wait for the next tick.
			continue
		}
		w.online[slug] = false
		wentOffline = append(wentOffline, slug)
	}
	subs := append([]chan string(nil), w.subs...)
	w.mu.Unlock()

	for _, slug := range wentOffline {
		w.logger.Info("agent went offline", "agent", slug)
		for _, ch := range subs {
			select {
			case ch <- slug:
			default:
			}
		}
	}
	return nil
}
