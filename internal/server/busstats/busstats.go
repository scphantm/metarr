// Package busstats samples the Redis instance backing the event bus into one
// shared in-process snapshot with a short rolling history, and fans that
// snapshot out to every dashboard viewer.
//
// The shape it produces is defined by proto — internal/server/busstats does
// not own the model, it aliases metarr.v1.BusSnapshot and friends, because
// that model crosses the wire to the dashboard (docs/adr/0005). See
// docs/adr/0007 for why the surface is what it is.
//
// One Sampler runs per server process. It polls Redis on a fixed interval
// (Run), keeps the last snapshot plus per-metric ring buffers under a mutex,
// and pushes the snapshot to every Stream subscriber on each pass. Get and
// Subscribe never touch Redis: a second dashboard adds no Redis load.
package busstats

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/timestamppb"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/eventbus"
)

// The snapshot model. Every type here aliases the generated metarr.v1
// message that defines it — proto is the single definition for a model that
// crosses a language boundary, and this one crosses the wire to the
// dashboard.
type (
	Snapshot     = metarrv1.BusSnapshot
	ServerInfo   = metarrv1.BusServerInfo
	StreamStat   = metarrv1.BusStreamStat
	GroupStat    = metarrv1.BusGroupStat
	ConsumerStat = metarrv1.BusConsumerStat
	ChannelStat  = metarrv1.BusChannelStat
)

const (
	// DefaultInterval is the sampler's polling cadence.
	DefaultInterval = 2 * time.Second
	// DefaultHistory is how many samples each per-metric ring buffer keeps:
	// 150 at the 2s cadence is roughly five minutes.
	DefaultHistory = 150
	// callTimeout bounds a single Redis call within a pass, so one slow call
	// cannot stall the cadence for the rest.
	callTimeout = 1500 * time.Millisecond
)

// infoBackedFields are the field_errors keys blanked together when the one
// INFO call in a pass fails. total_keys comes from a separate DBSIZE call and
// degrades on its own.
var infoBackedFields = []string{
	"version",
	"uptime_seconds",
	"connected_clients",
	"used_memory",
	"ops_per_second",
}

// RegisteredSlugsFunc yields the slugs of the registered agents — those
// configured on the server, online or not. The sampler unions each one's
// per-agent channels and command stream into the snapshot and cross-checks
// the live counts against them, so a crashed or disconnected agent shows as a
// flagged row rather than an absent one. In production it reads
// appconfig.Get(); a test passes a stub.
type RegisteredSlugsFunc func(ctx context.Context) []string

// Sampler polls Redis into one shared snapshot and fans it out to Stream
// subscribers. Construct it with New and drive it with Run.
type Sampler struct {
	client     redis.UniversalClient
	logger     *slog.Logger
	interval   time.Duration
	history    int
	slugSource RegisteredSlugsFunc

	mu         sync.Mutex
	snapshot   *Snapshot
	series     metricSeries
	streamHist map[string]*streamHistory
	groupHist  map[string]*groupHistory
	subs       map[chan *Snapshot]struct{}
}

// metricSeries is the set of per-metric ring buffers, oldest sample first.
type metricSeries struct {
	connectedClients []int64
	usedMemory       []int64
	opsPerSecond     []int64
	totalKeys        []int64
}

// streamHistory and groupHistory are the per-stream and per-group ring
// buffers, oldest sample first, keyed on the stream name and on
// groupSeriesKey respectively. Each also carries the previous pass's raw
// monotonic Redis counter, so the entry that owns a metric's history also
// owns the state its rate is derived from — the two are created and dropped
// together.
type streamHistory struct {
	length      []int64
	publishRate []int64

	prevEntriesAdded int64
	havePrevAdded    bool
}

type groupHistory struct {
	consumers     []int64
	pending       []int64
	lag           []int64
	oldestPending []int64
	consumeRate   []int64

	prevEntriesRead int64
	havePrevRead    bool
}

// Option configures a Sampler at construction.
type Option func(*Sampler)

// WithInterval overrides the polling cadence (default DefaultInterval).
func WithInterval(d time.Duration) Option {
	return func(s *Sampler) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithHistory overrides the ring-buffer depth (default DefaultHistory).
func WithHistory(n int) Option {
	return func(s *Sampler) {
		if n > 0 {
			s.history = n
		}
	}
}

// WithSlugSource gives the sampler the registered agent slugs it needs to
// derive the expected topology. Without it a pass still samples streams and
// channels, but no per-agent rows are unioned in and only the fixed known
// channels are cross-checked.
func WithSlugSource(fn RegisteredSlugsFunc) Option {
	return func(s *Sampler) { s.slugSource = fn }
}

// New wraps client as a Sampler. logger records a pass that reached no Redis;
// pass nil to discard it.
func New(client redis.UniversalClient, logger *slog.Logger, opts ...Option) *Sampler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	s := &Sampler{
		client:     client,
		logger:     logger,
		interval:   DefaultInterval,
		history:    DefaultHistory,
		streamHist: make(map[string]*streamHistory),
		groupHist:  make(map[string]*groupHistory),
		subs:       make(map[chan *Snapshot]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Prime takes one synchronous pass. Call it once before the server starts
// serving so Get never has to answer with a nil snapshot. Each Redis call in
// the pass is time-boxed, so an unreachable Redis delays startup by at most a
// couple of seconds rather than hanging it.
func (s *Sampler) Prime(ctx context.Context) {
	s.pass(ctx)
}

// Run takes one pass immediately, then one on every interval tick, until ctx
// is cancelled. Exactly one goroutine should call it per process.
func (s *Sampler) Run(ctx context.Context) {
	s.pass(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pass(ctx)
		}
	}
}

// Get returns the most recent snapshot without touching Redis. It is nil
// only in the brief window before the first pass completes.
func (s *Sampler) Get() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

// Subscribe registers a fan-out channel fed by the shared sampler loop and
// returns it with an unsubscribe func. The current snapshot, if any, is
// delivered immediately so a new Stream paints without waiting a full tick.
// The channel is buffered by one and lossy: a subscriber that falls behind
// skips to the next pass rather than back-pressuring the sampler.
//
// The channel is only ever closed by unsubscribe, and both the delete and
// the close happen under s.mu — the same lock pass() holds while it sends —
// so a fan-out send can never race a close.
func (s *Sampler) Subscribe() (<-chan *Snapshot, func()) {
	ch := make(chan *Snapshot, 1)

	s.mu.Lock()
	s.subs[ch] = struct{}{}
	if s.snapshot != nil {
		ch <- s.snapshot // non-blocking: the channel is fresh and buffered
	}
	s.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subs, ch)
			close(ch)
			s.mu.Unlock()
		})
	}
	return ch, unsubscribe
}

// pass takes one sample. A pass that reaches no Redis at all keeps the last
// snapshot in place — it ages out as stale on the dashboard — rather than
// replacing it with a blank one.
func (s *Sampler) pass(ctx context.Context) {
	server, reached := s.collectServer(ctx)
	if !reached {
		if ctx.Err() != nil {
			return // shutting down — not an infrastructure fault
		}
		s.logger.Warn("bus stats: sampler pass reached no Redis; keeping last snapshot")
		return
	}

	// The connection is up (collectServer reached it), so the stream reads
	// run outside the lock: a slow XINFO must not stall Get or the fan-out.
	// Each call inside is time-boxed on its own, and an unreadable stream
	// carries its own error rather than failing the pass.
	streams, published, consumed := s.collectStreams(ctx)
	channels := s.collectChannels(ctx)

	// The expected-vs-actual layer: derive what should be attached from the
	// stream-topic list and the registered agents, union in the rows Redis
	// did not surface (an offline agent's command stream and channels), and
	// flag every row whose live count falls short — naming which identity is
	// missing from the agent presence keys.
	var registered []string
	if s.slugSource != nil {
		registered = s.slugSource(ctx)
	}
	topology := DeriveTopology(eventbus.StreamTopics(), registered)
	present := s.collectPresence(ctx)
	streams = applyStreamTopology(streams, topology, present)
	channels = applyChannelTopology(channels, topology, present)

	s.mu.Lock()
	s.appendSeries(server)
	server.ConnectedClientsSeries = slices.Clone(s.series.connectedClients)
	server.UsedMemorySeries = slices.Clone(s.series.usedMemory)
	server.OpsPerSecondSeries = slices.Clone(s.series.opsPerSecond)
	server.TotalKeysSeries = slices.Clone(s.series.totalKeys)
	s.recordStreamMetrics(streams, published, consumed)

	snapshot := &Snapshot{
		CollectedAt: timestamppb.New(time.Now().UTC()),
		Server:      server,
		Streams:     streams,
		Channels:    channels,
	}
	s.snapshot = snapshot

	// Fan out under the lock. Each send is non-blocking (buffered by one,
	// drop on full), so holding s.mu across the loop costs nothing and keeps
	// a concurrent unsubscribe from closing a channel mid-send.
	for ch := range s.subs {
		select {
		case ch <- snapshot:
		default:
		}
	}
	s.mu.Unlock()
}

// appendSeries pushes this pass's values onto the ring buffers and trims each
// to the configured depth. A metric whose Redis call failed this pass carries
// its previous sample forward rather than recording a spurious zero, so a
// transient INFO failure does not punch a fake dip into the sparkline.
// Called with s.mu held.
func (s *Sampler) appendSeries(server *ServerInfo) {
	fe := server.GetFieldErrors()
	s.series.connectedClients = ringOrCarry(s.series.connectedClients, server.ConnectedClients, fe["connected_clients"] == "", s.history)
	s.series.usedMemory = ringOrCarry(s.series.usedMemory, server.UsedMemory, fe["used_memory"] == "", s.history)
	s.series.opsPerSecond = ringOrCarry(s.series.opsPerSecond, server.OpsPerSecond, fe["ops_per_second"] == "", s.history)
	s.series.totalKeys = ringOrCarry(s.series.totalKeys, server.TotalKeys, fe["total_keys"] == "", s.history)
}

// ringOrCarry appends value, or — when this metric's read failed this pass —
// repeats the last recorded sample. With no history yet and a failed read
// there is nothing to carry, so the pass simply is not recorded for that
// metric.
func ringOrCarry(buf []int64, value int64, ok bool, max int) []int64 {
	if !ok {
		if len(buf) == 0 {
			return buf
		}
		value = buf[len(buf)-1]
	}
	return ring(buf, value, max)
}

func ring(buf []int64, value int64, max int) []int64 {
	buf = append(buf, value)
	if len(buf) > max {
		buf = buf[len(buf)-max:]
	}
	return buf
}

// recordStreamMetrics appends every numeric stream and group metric from this
// pass to its ring buffer, derives the per-pass publish/consume rates from the
// raw Redis counters the pass read (published = each stream's total
// entries-added, consumed = each group's total entries-read), and clones the
// buffers onto the snapshot rows. A stream or group seen for the first time
// reports a zero rate rather than its whole lifetime count as a spike.
// History for streams and groups no longer in the topology is dropped.
// Called with s.mu held.
func (s *Sampler) recordStreamMetrics(streams []*StreamStat, published, consumed map[string]int64) {
	liveStreams := make(map[string]struct{}, len(streams))
	liveGroups := make(map[string]struct{})

	for _, st := range streams {
		liveStreams[st.Stream] = struct{}{}

		hist := s.streamHist[st.Stream]
		if hist == nil {
			hist = &streamHistory{}
			s.streamHist[st.Stream] = hist
		}

		if cur, ok := published[st.Stream]; ok {
			if hist.havePrevAdded {
				st.PublishRate = nonNegativeDelta(hist.prevEntriesAdded, cur)
			}
			hist.prevEntriesAdded, hist.havePrevAdded = cur, true
		}

		hist.length = ring(hist.length, st.Length, s.history)
		hist.publishRate = ring(hist.publishRate, st.PublishRate, s.history)
		st.LengthSeries = slices.Clone(hist.length)
		st.PublishRateSeries = slices.Clone(hist.publishRate)

		for _, g := range st.Groups {
			key := groupSeriesKey(st.Stream, g.Name)
			liveGroups[key] = struct{}{}

			gh := s.groupHist[key]
			if gh == nil {
				gh = &groupHistory{}
				s.groupHist[key] = gh
			}

			if cur, ok := consumed[key]; ok {
				if gh.havePrevRead {
					g.ConsumeRate = nonNegativeDelta(gh.prevEntriesRead, cur)
				}
				gh.prevEntriesRead, gh.havePrevRead = cur, true
			}

			gh.consumers = ring(gh.consumers, g.Consumers, s.history)
			gh.pending = ring(gh.pending, g.Pending, s.history)
			gh.lag = ring(gh.lag, g.Lag, s.history)
			gh.oldestPending = ring(gh.oldestPending, g.OldestPendingAgeSeconds, s.history)
			gh.consumeRate = ring(gh.consumeRate, g.ConsumeRate, s.history)
			g.ConsumersSeries = slices.Clone(gh.consumers)
			g.PendingSeries = slices.Clone(gh.pending)
			g.LagSeries = slices.Clone(gh.lag)
			g.OldestPendingAgeSecondsSeries = slices.Clone(gh.oldestPending)
			g.ConsumeRateSeries = slices.Clone(gh.consumeRate)
		}
	}

	for k := range s.streamHist {
		if _, ok := liveStreams[k]; !ok {
			delete(s.streamHist, k)
		}
	}
	for k := range s.groupHist {
		if _, ok := liveGroups[k]; !ok {
			delete(s.groupHist, k)
		}
	}
}

// nonNegativeDelta is cur-prev floored at zero, so a counter reset — Redis
// restarted, or the stream trimmed and recreated — reads as no activity for
// that pass rather than a large negative spike.
func nonNegativeDelta(prev, cur int64) int64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// collectServer fills BusServerInfo from INFO + DBSIZE. Each call is
// time-boxed on its own; a failed call blanks the fields it feeds and names
// each of them in field_errors. reached is false only when no call in the
// pass reached Redis at all.
func (s *Sampler) collectServer(ctx context.Context) (*ServerInfo, bool) {
	info := &ServerInfo{}
	fieldErrors := make(map[string]string)
	reached := false

	// One INFO with no section filter: the default set already carries every
	// field below (Server, Clients, Memory, Stats), and asking for named
	// sections buys nothing over parsing the ones we want out of the whole.
	infoCtx, cancelInfo := context.WithTimeout(ctx, callTimeout)
	raw, err := s.client.Info(infoCtx).Result()
	cancelInfo()
	if err != nil {
		for _, field := range infoBackedFields {
			fieldErrors[field] = err.Error()
		}
	} else {
		reached = true
		fields := parseInfo(raw)
		info.Version = fields["redis_version"]
		info.UptimeSeconds = infoInt(fields, "uptime_in_seconds")
		info.ConnectedClients = infoInt(fields, "connected_clients")
		info.UsedMemory = infoInt(fields, "used_memory")
		info.UsedMemoryHuman = fields["used_memory_human"]
		info.OpsPerSecond = infoInt(fields, "instantaneous_ops_per_sec")
	}

	sizeCtx, cancelSize := context.WithTimeout(ctx, callTimeout)
	keys, err := s.client.DBSize(sizeCtx).Result()
	cancelSize()
	if err != nil {
		fieldErrors["total_keys"] = err.Error()
	} else {
		reached = true
		info.TotalKeys = keys
	}

	if len(fieldErrors) > 0 {
		info.FieldErrors = fieldErrors
	}
	return info, reached
}
