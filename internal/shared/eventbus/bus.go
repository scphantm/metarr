package eventbus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Bus is the one durable-stream event bus (docs/adr/0008,
// docs/design/eventbus-bus-interface.md). It replaces the caller-assembled
// StreamBus + Router pair: one New, N HandleStream registrations, one
// Run(ctx) / Ready() / Close(). The envelope Source is stamped from process
// identity and the event Name is validated against the topic row, so a
// publish naming the wrong process or an off-table event is unrepresentable.
// Per-(topic, name) dispatch and the unknown-name default live here, once,
// instead of in every listener.
//
// The Pub/Sub half (HandleNotify / HandleRequest / Notify / Request) lives in
// bus_pubsub.go and holds cfg.Redis directly — no transport port. Run drives
// both halves under one call; Ready waits for both.
type Bus struct {
	cfg Config
	now func() time.Time

	// publisher is built in New and never reassigned, so Publish reads it
	// without the lock and works the instant New returns — there is no
	// Publish-before-Run window. Its MAXLEN cap is passed per call from the
	// late-bound policy, so it needs nothing from Run.
	publisher StreamPublisher

	mu            sync.Mutex
	registrations map[string]streamRegistration  // keyed by topic.Name
	notifyRegs    []notifyRegistration           // KindNotify; many per topic
	requestRegs   map[string]requestRegistration // KindRequestReply; keyed by topic.Name, one per topic
	started       bool
	router        *message.Router

	ready     chan struct{}
	readyOnce sync.Once
}

// Config is everything the Bus needs. Built once, before bootstrap.
type Config struct {
	// Redis is the shared client, used here only by the retention sweep's
	// admin ops (XTRIM, SCAN). The Bus NEVER closes it. May be nil when
	// RetentionSweep is false and the transport is ChannelStreamTransport.
	Redis redis.UniversalClient

	// Source is stamped on every envelope this process publishes
	// (SourceServer or AgentSource(slug)) and DERIVES the durable-stream
	// consumer identity. No call site ever names it.
	Source string

	// Streams is the durable-stream transport port. Production:
	// RedisStreamTransport. Tests: ChannelStreamTransport.
	Streams StreamTransport

	// Policy is the late-bound tuning provider — never nil. Read once per
	// publish for the MAXLEN cap, and once in Run for the retry stack and for
	// the retention sweep's window + interval (the sweeper is built from that
	// one snapshot, so a live retention change is only picked up on the next
	// Run). Before the event_bus config section is bootstrapped it returns
	// DefaultBusPolicy(); after, the server's closure returns live values.
	// This is what collapses the server's build-twice startup.
	Policy func() BusPolicy

	Logger *slog.Logger

	// RetentionSweep runs the age-based XTRIM sweep inside Run. Server: true.
	// Agent: false (no canonical view to trim).
	RetentionSweep bool

	// Now stamps envelope timestamps and the retention cutoff. Zero value →
	// time.Now().UTC().
	Now func() time.Time
}

// Errors, all errors.Is-matchable. ErrNoResponder lives in pubsub.go and is
// unchanged.
var (
	// ErrBusRunning is returned by a registration call made after Run, and
	// by a second Run.
	ErrBusRunning = errors.New("eventbus: bus is already running; register before Run")
	// ErrUnknownEvent is returned when an event name is not in the topic
	// row's Events list — a publish or a handler registration naming it is
	// rejected before any Redis call.
	ErrUnknownEvent = errors.New("eventbus: event name is not legal on this topic")
	// ErrWrongKind is returned when a Topic of the wrong Kind is passed to a
	// verb — a KindNotify row to Publish, say.
	ErrWrongKind = errors.New("eventbus: topic is the wrong kind for this operation")
	// ErrNotPublishable is returned by Publish for a pattern topic or a name
	// the stream topic table does not resolve.
	ErrNotPublishable = errors.New("eventbus: topic is not publishable")
)

type streamRegistration struct {
	topic    Topic
	handlers map[string]StreamHandler
}

// New validates cfg and returns a Bus with nothing registered and nothing
// running.
func New(cfg Config) (*Bus, error) {
	switch {
	case cfg.Source == "":
		return nil, errors.New("eventbus: Config.Source is required")
	case cfg.Streams == nil:
		return nil, errors.New("eventbus: Config.Streams is required")
	case cfg.Policy == nil:
		return nil, errors.New("eventbus: Config.Policy is required")
	case cfg.Logger == nil:
		return nil, errors.New("eventbus: Config.Logger is required")
	}

	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	publisher, err := cfg.Streams.Publisher()
	if err != nil {
		return nil, fmt.Errorf("eventbus: stream publisher: %w", err)
	}

	return &Bus{
		cfg:           cfg,
		now:           now,
		publisher:     publisher,
		registrations: map[string]streamRegistration{},
		requestRegs:   map[string]requestRegistration{},
		ready:         make(chan struct{}),
	}, nil
}

// HandleStream registers the sole consumer of a KindStream topic. The map key
// is the envelope Name; dispatch is per (topic, Name). Every key MUST be in
// topic.Events. An event whose Name is on the stream but not in the map hits
// the unknown-name default — logged once, then acked — which lives in exactly
// one place, dispatch below. All registration must precede Run.
func (b *Bus) HandleStream(topic Topic, handlers map[string]StreamHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return ErrBusRunning
	}
	if topic.Kind != KindStream {
		return fmt.Errorf("%w: HandleStream needs a %s topic, got %q on %q", ErrWrongKind, KindStream, topic.Kind, topic.Name)
	}
	if topic.Pattern {
		return fmt.Errorf("%w: %q is a pattern topic", ErrNotPublishable, topic.Name)
	}
	if topic.Group == "" {
		return fmt.Errorf("eventbus: stream topic %q has no consumer group to read with", topic.Name)
	}
	if len(handlers) == 0 {
		return fmt.Errorf("eventbus: HandleStream on %q needs at least one handler", topic.Name)
	}
	for name := range handlers {
		if !slices.Contains(topic.Events, name) {
			return fmt.Errorf("%w: %q on stream %q", ErrUnknownEvent, name, topic.Name)
		}
	}
	if _, dup := b.registrations[topic.Name]; dup {
		return fmt.Errorf("eventbus: stream topic %q already has a handler map", topic.Name)
	}

	owned := make(map[string]StreamHandler, len(handlers))
	for name, handler := range handlers {
		owned[name] = handler
	}
	b.registrations[topic.Name] = streamRegistration{topic: topic, handlers: owned}
	return nil
}

// Publish appends one event to a KindStream topic. The caller supplies name,
// correlationID, payload; the Bus stamps Source from cfg.Source (unforgeable
// at the call site) and Timestamp from cfg.Now(). name MUST be in
// topic.Events, checked before any Redis call. Non-blocking. correlationID is
// explicit — the Bus does not read it from ctx.
func (b *Bus) Publish(ctx context.Context, topic Topic, name, correlationID string, payload []byte) error {
	if topic.Kind != KindStream {
		return fmt.Errorf("%w: Publish needs a %s topic, got %q on %q", ErrWrongKind, KindStream, topic.Kind, topic.Name)
	}
	if err := streamTopicPublishable(topic); err != nil {
		return fmt.Errorf("%w: %w", ErrNotPublishable, err)
	}
	if !slices.Contains(topic.Events, name) {
		return fmt.Errorf("%w: %q on stream %q", ErrUnknownEvent, name, topic.Name)
	}

	encoded, err := MarshalEvent(&Event{
		Name:          name,
		Source:        b.cfg.Source,
		CorrelationId: correlationID,
		Timestamp:     timestamppb.New(b.now()),
		Payload:       payload,
	})
	if err != nil {
		return err
	}
	// MAXLEN is read from the late-bound policy per publish, the same way the
	// retention sweep reads it per iteration.
	return b.publisher.Publish(ctx, topic.Name, encoded, b.cfg.Policy().Retention.MaxLen)
}

// Run builds the Watermill router from the late-bound policy, starts every
// registered handler and — when cfg.RetentionSweep — the age-based trim
// sweep, and blocks until ctx is cancelled or the router exits. All
// registration must precede it. At most one call.
//
// A setup failure here (router or subscriber construction) is returned; it
// leaves the Bus un-started so the caller can decide whether to escalate.
// The stream publisher is already live from New, so a returned error never
// affects Publish.
func (b *Bus) Run(ctx context.Context) (err error) {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return ErrBusRunning
	}
	b.started = true
	regs := make([]streamRegistration, 0, len(b.registrations))
	for _, reg := range b.registrations {
		regs = append(regs, reg)
	}
	b.mu.Unlock()

	// On any exit — setup error or a clean stop — never leave a Ready()
	// waiter blocked, and let a setup failure be retried.
	defer func() {
		b.readyOnce.Do(func() { close(b.ready) })
		if err != nil {
			b.mu.Lock()
			b.started = false
			b.mu.Unlock()
		}
	}()

	policy := b.cfg.Policy()

	adapter := NewSlogAdapter(b.cfg.Logger)
	router, err := message.NewRouter(message.RouterConfig{}, adapter)
	if err != nil {
		return fmt.Errorf("eventbus: new router: %w", err)
	}
	router.AddMiddleware(
		middleware.Recoverer,
		dropAfterRetry(adapter),
		middleware.Retry{
			MaxRetries:      policy.Retry.MaxAttempts,
			InitialInterval: policy.Retry.BackoffBase,
			MaxInterval:     policy.Retry.BackoffMax,
			Multiplier:      retryBackoffMultiplier,
			Logger:          adapter,
		}.Middleware,
	)

	consumer := b.consumerName()
	for _, reg := range regs {
		subscriber, subErr := b.cfg.Streams.Subscriber(reg.topic.Group, consumer)
		if subErr != nil {
			return fmt.Errorf("eventbus: subscriber for %s: %w", reg.topic.Name, subErr)
		}
		router.AddNoPublisherHandler(reg.topic.Name, reg.topic.Name, subscriber, b.dispatch(reg))
	}

	b.mu.Lock()
	b.router = router
	b.mu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The Pub/Sub half: one subscription per notify handler and per
	// request/reply responder, every SUBSCRIBE acknowledged before we return.
	// It runs before the stream router starts, so Ready — gated below on the
	// router reporting itself running — cannot close until these are live too.
	// A subscribe failure here is a setup failure, returned like a stream
	// subscriber failure so the Bus stays re-runnable.
	var pubsub sync.WaitGroup
	subs, err := b.startPubSub(runCtx, &pubsub)
	if err != nil {
		return err
	}

	var sweep sync.WaitGroup
	if b.cfg.RetentionSweep {
		sweeper := NewRetentionSweeper(b.cfg.Redis, policy.Retention, policy.SweepInterval, b.cfg.Logger)
		sweeper.now = b.now
		sweep.Add(1)
		go func() {
			defer sweep.Done()
			sweeper.Run(runCtx)
		}()
	}

	go func() {
		select {
		case <-router.Running():
			b.readyOnce.Do(func() { close(b.ready) })
		case <-runCtx.Done():
		}
	}()

	// message.Router.Run only unblocks on router.Close(), not on context
	// cancellation — and with no handlers registered it never self-closes.
	// So translate ctx cancellation into a Close here.
	go func() {
		<-runCtx.Done()
		_ = router.Close()
	}()

	runErr := router.Run(runCtx)
	cancel()

	// runCtx is cancelled now (router.Run only returns after it is, or on a
	// setup error we then cancel). Close every Pub/Sub subscription and wait
	// for its receive loop to drain, mirroring sweep.Wait().
	for _, sub := range subs {
		_ = sub.Close()
	}
	pubsub.Wait()
	sweep.Wait()
	return runErr
}

// dispatch decodes the envelope once and indexes the registered map by
// event.Name. A decode failure is returned as an error (retried a few times,
// then logged and dropped by the middleware). An index miss is the
// unknown-name default: logged once with stream + name, then acked.
func (b *Bus) dispatch(reg streamRegistration) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var event Event
		if err := UnmarshalEvent(msg.Payload, &event); err != nil {
			return fmt.Errorf("eventbus: decode envelope on %s: %w", reg.topic.Name, err)
		}

		handler, ok := reg.handlers[event.Name]
		if !ok {
			b.cfg.Logger.Warn("eventbus: no handler for stream event",
				"stream", reg.topic.Name,
				"name", event.Name,
				"correlation_id", event.CorrelationId,
			)
			return nil
		}
		return handler(msg.Context(), &event)
	}
}

// consumerName derives the durable-stream consumer identity from cfg.Source:
// SourceServer → the one server consumer name; AgentSource(slug) → slug.
func (b *Bus) consumerName() string {
	if b.cfg.Source == SourceServer {
		return ConsumerName
	}
	if slug, ok := SlugFromAgentSource(b.cfg.Source); ok {
		return slug
	}
	return b.cfg.Source
}

// Ready is closed once every stream handler is live AND every Pub/Sub
// SUBSCRIBE is acknowledged — or once Run returns, so a waiter is never left
// blocked by an early shutdown. The Pub/Sub subscriptions are acknowledged
// synchronously in Run before the router starts, so gating this on the router
// reporting itself running covers both halves. A warm-up publisher waits on
// it. (Publish itself needs no warm-up: the publisher is live from New.)
func (b *Bus) Ready() <-chan struct{} { return b.ready }

// Close tears down the router and the stream publisher. The Pub/Sub receive
// loops and their subscriptions are bound to Run's context and stop when it is
// cancelled. It NEVER closes cfg.Redis.
func (b *Bus) Close() error {
	b.mu.Lock()
	router, publisher := b.router, b.publisher
	b.mu.Unlock()

	var errs []error
	if router != nil {
		errs = append(errs, router.Close())
	}
	if publisher != nil {
		errs = append(errs, publisher.Close())
	}
	return errors.Join(errs...)
}
