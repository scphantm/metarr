package eventbus

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
)

// PubSubNotifyHandler consumes one notification. It is handed the raw payload
// bytes published on the channel — no envelope decode, because a notification
// carries whatever its publisher put on the wire. A PubSubRouter.Handle
// registration owns the receive guard, the channel loop, and the shutdown of
// its subscription; the handler only processes a payload.
type PubSubNotifyHandler func(ctx context.Context, payload []byte)

// PubSubRespondHandler is the answering side of a request/reply exchange. It
// receives the decoded request envelope and returns the reply payload. A
// non-nil replyPayload is wrapped in a reply envelope by the router and
// published on the request's correlation-scoped reply channel; a nil
// replyPayload means send no reply. A returned error is logged at error level
// and produces no reply, so the caller hits the existing ErrNoResponder
// timeout.
type PubSubRespondHandler func(ctx context.Context, request *Event) (replyPayload []byte, err error)

// pubsubRegistration is one accumulated Handle or Respond call. Exactly one of
// notify / respond is set; replyName is meaningful only for a respond
// registration.
type pubsubRegistration struct {
	channel   string
	replyName string
	notify    PubSubNotifyHandler
	respond   PubSubRespondHandler
}

// PubSubRouter is the Pub/Sub counterpart of the stream Router (docs/adr/0006).
// Every notification subscriber and every answering side of request/reply in a
// process runs through one PubSubRouter: registrations accumulate through
// Handle / Respond, then a single Run(ctx) opens one Redis subscription per
// registration and drives its receive loop until ctx is cancelled. Running()
// closes once every subscription is live, matching the stream side.
//
// There is no retry and no drop-after-retry here: a Handle error is the
// handler's own to swallow or log, a Respond error is logged at error level
// and nothing else, and a failed Respond simply sends no reply so the caller
// hits its existing ErrNoResponder timeout.
type PubSubRouter struct {
	bus    *PubSubBus
	source string
	logger *slog.Logger

	mu            sync.Mutex
	registrations []pubsubRegistration
	started       bool

	running chan struct{}
}

// NewPubSubRouter builds a router over client. source is the envelope Source
// stamped on every reply this router publishes — SourceServer in the server,
// AgentSource(slug) in an agent — so a responder cannot answer as the wrong
// process.
func NewPubSubRouter(client *redis.Client, source string, logger *slog.Logger) *PubSubRouter {
	return &PubSubRouter{
		bus:     NewPubSubBus(client),
		source:  source,
		logger:  logger,
		running: make(chan struct{}),
	}
}

// Handle registers handler as a notification consumer on channel. Multiple
// handlers may be registered on one channel; each opens its own subscription.
// Registrations must be added before Run is called.
func (r *PubSubRouter) Handle(channel string, handler PubSubNotifyHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations = append(r.registrations, pubsubRegistration{channel: channel, notify: handler})
}

// Respond registers handler as the answering side of request/reply on channel.
// The router decodes the request envelope, calls handler, and on a non-nil
// reply payload builds the reply envelope — stamping this router's source, the
// request's correlation ID, and replyName — then publishes it on the request's
// correlation-scoped reply channel. A nil reply payload or a handler error
// sends no reply. Registrations must be added before Run is called.
func (r *PubSubRouter) Respond(channel, replyName string, handler PubSubRespondHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations = append(r.registrations, pubsubRegistration{
		channel:   channel,
		replyName: replyName,
		respond:   handler,
	})
}

// Run opens a subscription per registration, waits for every SUBSCRIBE to be
// acknowledged, closes Running(), and then blocks until ctx is cancelled. On
// cancellation it closes every subscription and waits for the receive loops to
// return. It may be called once per router.
func (r *PubSubRouter) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return errors.New("eventbus: PubSubRouter already running")
	}
	r.started = true
	regs := make([]pubsubRegistration, len(r.registrations))
	copy(regs, r.registrations)
	r.mu.Unlock()

	subs := make([]*redis.PubSub, 0, len(regs))
	var wg sync.WaitGroup

	for _, reg := range regs {
		// Same low-level subscribe path Request uses — the unexported
		// PubSubBus.subscribe, the only way to open a Pub/Sub subscription
		// left in this package.
		sub := r.bus.subscribe(ctx, reg.channel)

		// Wait for the subscription to be acknowledged before declaring the
		// router live, so a publish that races Run cannot slip past the loop.
		if _, err := sub.Receive(ctx); err != nil {
			_ = sub.Close()
			for _, opened := range subs {
				_ = opened.Close()
			}
			return err
		}
		subs = append(subs, sub)

		wg.Add(1)
		go func(reg pubsubRegistration, sub *redis.PubSub) {
			defer wg.Done()
			r.consume(ctx, reg, sub)
		}(reg, sub)
	}

	close(r.running)

	<-ctx.Done()

	for _, sub := range subs {
		_ = sub.Close()
	}
	wg.Wait()
	return nil
}

// Running is closed once Run has every subscription live. Callers that must
// publish only after the consumers exist — tests, warm-up events — wait on it.
func (r *PubSubRouter) Running() chan struct{} {
	return r.running
}

// consume is the per-registration receive loop the seam owns so a handler is
// one registration call and never re-implements it.
func (r *PubSubRouter) consume(ctx context.Context, reg pubsubRegistration, sub *redis.PubSub) {
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			if reg.notify != nil {
				reg.notify(ctx, []byte(msg.Payload))
				continue
			}
			r.dispatchRespond(ctx, reg, msg.Payload)
		}
	}
}

// dispatchRespond decodes one request, runs the handler, and publishes the
// reply envelope when the handler returned a payload.
func (r *PubSubRouter) dispatchRespond(ctx context.Context, reg pubsubRegistration, payload string) {
	var request Event
	if err := UnmarshalEvent([]byte(payload), &request); err != nil {
		r.logger.Error("pubsub responder could not decode request", "channel", reg.channel, "error", err)
		return
	}

	replyPayload, err := reg.respond(ctx, &request)
	if err != nil {
		r.logger.Error("pubsub responder failed",
			"channel", reg.channel,
			"reply_name", reg.replyName,
			"correlation_id", request.GetCorrelationId(),
			"error", err)
		return
	}
	if replyPayload == nil {
		return
	}

	reply := NewEvent(r.source, reg.replyName, request.GetCorrelationId(), replyPayload)
	if err := r.bus.Reply(ctx, request.GetCorrelationId(), reply); err != nil {
		r.logger.Error("pubsub responder could not publish reply",
			"channel", reg.channel,
			"reply_name", reg.replyName,
			"correlation_id", request.GetCorrelationId(),
			"error", err)
	}
}
