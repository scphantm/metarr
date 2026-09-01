package busstats

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/redis/go-redis/v9"
)

func newTestSampler(t *testing.T) (*Sampler, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return New(client, nil, WithHistory(4)), mr
}

// A pass against a reachable Redis fills the shape: collected_at is stamped,
// the counters Redis reports land on the snapshot, and no field is flagged.
func TestPassPopulatesSnapshotFromRedis(t *testing.T) {
	sampler, mr := newTestSampler(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := mr.Set("key"+string(rune('a'+i)), "v"); err != nil {
			t.Fatalf("seed key: %v", err)
		}
	}

	sampler.pass(ctx)

	snap := sampler.Get()
	if snap == nil {
		t.Fatal("Get returned nil after a pass")
	}
	if snap.GetCollectedAt() == nil {
		t.Error("collected_at was not stamped")
	}
	if snap.GetServer() == nil {
		t.Fatal("server info is nil")
	}
	if len(snap.GetServer().GetFieldErrors()) != 0 {
		t.Errorf("clean pass reported field errors: %v", snap.GetServer().GetFieldErrors())
	}
	if got := snap.GetServer().GetTotalKeys(); got != 3 {
		t.Errorf("total_keys = %d, want 3", got)
	}
	if got := snap.GetServer().GetConnectedClients(); got < 1 {
		t.Errorf("connected_clients = %d, want at least 1", got)
	}
	// Streams and channels are stubbed empty in the walking skeleton, but the
	// fields must be present and non-nil so the dashboard can render a table.
	if snap.GetStreams() == nil || snap.GetChannels() == nil {
		t.Error("streams/channels slices should be empty, not nil")
	}
}

// Each pass appends one sample to every per-metric ring buffer, and the
// buffer is capped at the configured depth.
func TestPassAppendsBoundedHistory(t *testing.T) {
	sampler, _ := newTestSampler(t) // WithHistory(4)
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		sampler.pass(ctx)
	}

	series := sampler.Get().GetServer().GetTotalKeysSeries()
	if len(series) != 4 {
		t.Fatalf("total_keys_series length = %d, want 4 (capped)", len(series))
	}
	if len(sampler.Get().GetServer().GetOpsPerSecondSeries()) != 4 {
		t.Errorf("ops_per_second_series was not capped to the history depth")
	}
}

// One INFO call failing blanks only the fields it feeds and records a
// per-field error for each; DBSIZE still succeeds, so total_keys is intact
// and the pass still produces a snapshot.
func TestPassDegradesIndividualFieldsWhenInfoFails(t *testing.T) {
	sampler, mr := newTestSampler(t)
	ctx := context.Background()

	sampler.pass(ctx) // one good pass first

	mr.Server().SetPreHook(func(c *server.Peer, cmd string, args ...string) bool {
		if strings.EqualFold(cmd, "INFO") {
			c.WriteError("LOADING Redis is loading the dataset in memory")
			return true
		}
		return false
	})

	if err := mr.Set("k1", "v"); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	sampler.pass(ctx)

	server := sampler.Get().GetServer()
	for _, field := range infoBackedFields {
		if _, ok := server.GetFieldErrors()[field]; !ok {
			t.Errorf("field_errors missing an entry for %q", field)
		}
	}
	if _, ok := server.GetFieldErrors()["total_keys"]; ok {
		t.Error("total_keys was flagged even though DBSIZE succeeded")
	}
	if got := server.GetTotalKeys(); got != 1 {
		t.Errorf("total_keys = %d, want 1 (DBSIZE still worked)", got)
	}

	// The failed INFO must not punch a fake 0 into the sparkline: the
	// connected-clients series carries its last real sample forward, while
	// total_keys — read by the DBSIZE call that still worked — advances.
	cc := server.GetConnectedClientsSeries()
	if n := len(cc); n < 2 || cc[n-1] != cc[n-2] {
		t.Errorf("connected_clients_series did not carry forward on INFO failure: %v", cc)
	}
	if tk := server.GetTotalKeysSeries(); len(tk) < 1 || tk[len(tk)-1] != 1 {
		t.Errorf("total_keys_series should record the live DBSIZE reading: %v", tk)
	}
}

// Losing Redis entirely does not replace the last good snapshot with a blank
// one, and Get keeps serving it without touching Redis.
func TestGetKeepsLastSnapshotAfterTotalLoss(t *testing.T) {
	sampler, mr := newTestSampler(t)
	ctx := context.Background()

	sampler.pass(ctx)
	before := sampler.Get()
	if before.GetServer().GetTotalKeys() != 0 {
		t.Fatalf("unexpected seed state")
	}

	mr.Close()
	sampler.pass(ctx) // reaches nothing

	after := sampler.Get()
	if after != before {
		t.Error("a pass that reached no Redis replaced the last snapshot")
	}
	if after == nil {
		t.Error("Get returned nil after total loss")
	}
}

// The sampler runs one loop no matter how many Stream clients subscribe:
// Subscribe issues no Redis command, and a pass with many subscribers costs
// the same Redis traffic as a pass with none.
func TestSubscribeAddsNoRedisLoad(t *testing.T) {
	sampler, mr := newTestSampler(t)
	ctx := context.Background()

	sampler.pass(ctx) // warm the connection so deltas are stable

	baseline := mr.Server().TotalCommands()
	sampler.pass(ctx)
	perPass := mr.Server().TotalCommands() - baseline
	if perPass == 0 {
		t.Fatal("a pass issued no Redis commands; test cannot measure fan-out")
	}

	var unsubs []func()
	var chans []<-chan *Snapshot
	for i := 0; i < 5; i++ {
		ch, unsub := sampler.Subscribe()
		chans = append(chans, ch)
		unsubs = append(unsubs, unsub)
	}
	defer func() {
		for _, unsub := range unsubs {
			unsub()
		}
	}()

	if got := mr.Server().TotalCommands() - baseline - perPass; got != 0 {
		t.Errorf("5 Subscribe calls issued %d Redis commands, want 0", got)
	}

	// Drain the immediate first-paint frame each subscriber got on connect.
	for _, ch := range chans {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive the first-paint snapshot")
		}
	}

	before := mr.Server().TotalCommands()
	sampler.pass(ctx)
	if got := mr.Server().TotalCommands() - before; got != perPass {
		t.Errorf("a pass with 5 subscribers cost %d commands, want %d (no per-connection Redis work)", got, perPass)
	}

	for _, ch := range chans {
		select {
		case snap := <-ch:
			if snap == nil {
				t.Error("subscriber received a nil snapshot")
			}
		case <-time.After(time.Second):
			t.Error("subscriber did not receive the pass snapshot")
		}
	}
}

// Unsubscribe closes the channel and is safe to call more than once.
func TestUnsubscribeIsIdempotent(t *testing.T) {
	sampler, _ := newTestSampler(t)

	ch, unsub := sampler.Subscribe()
	unsub()
	unsub()

	if _, open := <-ch; open {
		t.Error("channel should be closed after unsubscribe")
	}
}

// A subscriber unsubscribing while a pass is fanning out must not race the
// close against the send. Run under -race to mean anything.
func TestConcurrentUnsubscribeDuringFanOut(t *testing.T) {
	sampler, _ := newTestSampler(t)
	ctx := context.Background()
	sampler.pass(ctx)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			sampler.pass(ctx)
		}
		close(done)
	}()

	for i := 0; i < 200; i++ {
		_, unsub := sampler.Subscribe()
		unsub()
	}
	<-done
}

// Run takes a pass immediately rather than waiting a full interval, so the
// first dashboard has something within milliseconds.
func TestRunSamplesImmediately(t *testing.T) {
	sampler, _ := newTestSampler(t)
	sampler.interval = time.Hour // guarantee the first pass is the immediate one

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sampler.Run(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if sampler.Get() != nil {
			return
		}
		select {
		case <-deadline:
			t.Fatal("Run did not produce a snapshot promptly")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
