package agentregistry

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"Metarr/internal/shared/agentproto"
)

// fakePresenceLister returns whatever slugs the test currently wants present.
type fakePresenceLister struct {
	slugs []string
	err   error
}

func (f *fakePresenceLister) PresentSlugs(context.Context) ([]string, error) {
	return f.slugs, f.err
}

// fakeClock is a hand-advanced clock.
type fakeClock struct{ now time.Time }

func (c *fakeClock) time() time.Time { return c.now }
func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newTestWatcher(t *testing.T) (*PresenceWatcher, *fakePresenceLister, *fakeClock, <-chan string) {
	t.Helper()
	lister := &fakePresenceLister{}
	clock := &fakeClock{now: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
	watcher := NewPresenceWatcher(lister, clock.time, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return watcher, lister, clock, watcher.Subscribe()
}

func scan(t *testing.T, w *PresenceWatcher) {
	t.Helper()
	if err := w.scanOnce(context.Background()); err != nil {
		t.Fatalf("scanOnce: %v", err)
	}
}

func expectOffline(t *testing.T, signals <-chan string, want string) {
	t.Helper()
	select {
	case got := <-signals:
		if got != want {
			t.Fatalf("offline signal for %q, want %q", got, want)
		}
	default:
		t.Fatalf("expected an offline signal for %q, got none", want)
	}
}

func expectNoSignal(t *testing.T, signals <-chan string) {
	t.Helper()
	select {
	case got := <-signals:
		t.Fatalf("expected no offline signal, got one for %q", got)
	default:
	}
}

func TestPresenceWatcherEmitsOnceWhenAKeyVanishesPastTTL(t *testing.T) {
	watcher, lister, clock, signals := newTestWatcher(t)

	lister.slugs = []string{"nas-01"}
	scan(t, watcher)
	expectNoSignal(t, signals)

	// Key gone. Within the TTL grace this is a missed scan, not a departure.
	lister.slugs = nil
	clock.advance(agentproto.PresenceTTL / 2)
	scan(t, watcher)
	expectNoSignal(t, signals)

	// Still gone, now past the TTL: offline, exactly once.
	clock.advance(agentproto.PresenceTTL)
	scan(t, watcher)
	expectOffline(t, signals, "nas-01")

	// A further scan with the key still absent must not re-emit.
	clock.advance(agentproto.PresenceTTL)
	scan(t, watcher)
	expectNoSignal(t, signals)
}

func TestPresenceWatcherEmitsAgainAfterAKeyReappearsAndVanishes(t *testing.T) {
	watcher, lister, clock, signals := newTestWatcher(t)

	lister.slugs = []string{"nas-01"}
	scan(t, watcher)

	lister.slugs = nil
	clock.advance(2 * agentproto.PresenceTTL)
	scan(t, watcher)
	expectOffline(t, signals, "nas-01")

	// Back online.
	lister.slugs = []string{"nas-01"}
	clock.advance(time.Second)
	scan(t, watcher)
	expectNoSignal(t, signals)

	// Gone again, past the TTL: a second signal.
	lister.slugs = nil
	clock.advance(2 * agentproto.PresenceTTL)
	scan(t, watcher)
	expectOffline(t, signals, "nas-01")
}

func TestPresenceWatcherStaysQuietWhileTheKeyIsPresent(t *testing.T) {
	watcher, lister, clock, signals := newTestWatcher(t)

	lister.slugs = []string{"nas-01"}
	for range 10 {
		clock.advance(agentproto.PresenceTTL)
		scan(t, watcher)
	}
	expectNoSignal(t, signals)
}

func TestPresenceWatcherFansOutToEverySubscriber(t *testing.T) {
	watcher, lister, clock, first := newTestWatcher(t)
	second := watcher.Subscribe()

	lister.slugs = []string{"nas-01"}
	scan(t, watcher)
	lister.slugs = nil
	clock.advance(2 * agentproto.PresenceTTL)
	scan(t, watcher)

	expectOffline(t, first, "nas-01")
	expectOffline(t, second, "nas-01")
}
