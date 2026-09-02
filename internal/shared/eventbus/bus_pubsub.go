package eventbus

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/correlation"
)

// This file is the Pub/Sub half of the Bus: fire-and-forget notifications
// (HandleNotify / Notify) and the one synchronous request/reply pattern
// (HandleRequest / Request). It runs on the PubSubTransport seam
// (docs/design/eventbus-bus-interface.md §4 amendment): production leaves
// Config.PubSub nil and New defaults it to RedisPubSub over the shared
// client, so the wire behaviour is unchanged; tests pass InMemoryPubSub().
// The SUBSCRIBE-ack-before-publish ordering and the correlation-scoped reply
// routing stay here, on the Bus side of the seam — the transport only opens
// subscriptions and publishes bytes.
//
// The receive loops are driven by Run, alongside the durable-stream router,
// under one call; Ready waits for both (every stream handler live AND every
// SUBSCRIBE acknowledged).

// ErrNoResponder is returned by Request when no reply comes back — the
// timeout elapsed with nothing listening on the request channel or the
// responder too slow, or the reply subscription closed under it. On the
// timeout path (the common one) it also wraps the context's deadline error,
// so errors.Is(err, context.DeadlineExceeded) still matches there.
var ErrNoResponder = errors.New("eventbus: no responder answered the request")

// errNoPubSub is returned by the Pub/Sub verbs when the Bus has no transport
// for that half — neither Config.PubSub nor Config.Redis was set.
var errNoPubSub = errors.New("eventbus: Pub/Sub needs Config.PubSub or Config.Redis")

// ReplyChannel returns the per-request reply channel name for a correlation
// ID. Scoping the reply channel to the correlation ID lets many concurrent
// callers share the same request channel without stealing each other's
// replies.
func ReplyChannel(correlationID string) string {
	return fmt.Sprintf("reply.%s", correlationID)
}

// RequestHandler is the answering side of a request/reply exchange. It
// receives the decoded request envelope and returns the reply payload. On a
// non-nil replyPayload the Bus assembles the reply envelope — Source from
// cfg.Source, correlation id copied from the request, Name from the topic's
// ReplyName — and publishes it on the correlation-scoped reply channel. A nil
// replyPayload OR a returned error sends no reply, so the caller hits
// ErrNoResponder.
type RequestHandler func(ctx context.Context, request *Event) (replyPayload []byte, err error)

// NotifyHandler consumes one notification. It is handed the raw payload bytes
// published on the channel — never an envelope decode, because a notification
// carries whatever its publisher put on the wire (LogTopic ships raw slog
// records). The Bus owns the subscription, the receive loop, and shutdown; the
// handler only processes a payload.
type NotifyHandler func(ctx context.Context, payload []byte)

type notifyRegistration struct {
	topic   Topic
	handler NotifyHandler
}

type requestRegistration struct {
	topic   Topic
	handler RequestHandler
}

// HandleNotify registers a fire-and-forget consumer for a notify topic.
// Payload is opaque bytes — never decoded. Multiple handlers may share one
// topic; each opens its own subscription. All registration must precede Run.
func (b *Bus) HandleNotify(topic NotifyTopic, handler NotifyHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return ErrBusRunning
	}
	if handler == nil {
		return fmt.Errorf("eventbus: HandleNotify on %q needs a handler", topic.Name)
	}
	if b.pubsub == nil {
		return errNoPubSub
	}
	b.notifyRegs = append(b.notifyRegs, notifyRegistration{topic: topic.Topic, handler: handler})
	return nil
}

// HandleRequest registers the answering side for a request/reply topic. The
// Bus decodes the request envelope, calls handler, and on a non-nil payload
// assembles the reply (Source = cfg.Source, correlation id copied from the
// request, Name = topic.ReplyName) and publishes it on
// ReplyChannel(correlation_id). A nil payload OR a handler error sends no
// reply, so the caller hits ErrNoResponder. Requires topic.ReplyName != "".
// One responder per topic; all registration must precede Run.
func (b *Bus) HandleRequest(topic RequestTopic, handler RequestHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return ErrBusRunning
	}
	if topic.ReplyName == "" {
		return fmt.Errorf("eventbus: request/reply topic %q has no ReplyName to stamp on the reply", topic.Name)
	}
	if handler == nil {
		return fmt.Errorf("eventbus: HandleRequest on %q needs a handler", topic.Name)
	}
	if b.pubsub == nil {
		return errNoPubSub
	}
	if _, dup := b.requestRegs[topic.Name]; dup {
		return fmt.Errorf("eventbus: request/reply topic %q already has a responder", topic.Name)
	}
	b.requestRegs[topic.Name] = requestRegistration{topic: topic.Topic, handler: handler}
	return nil
}

// Notify publishes opaque bytes on a notify topic. No envelope, no retry,
// at-most-once. Non-blocking.
func (b *Bus) Notify(ctx context.Context, topic NotifyTopic, payload []byte) error {
	if b.pubsub == nil {
		return errNoPubSub
	}
	return b.pubsub.Publish(ctx, topic.Name, payload)
}

// Request publishes a request envelope (Source = cfg.Source, the given name,
// the given correlationID — minted when "") on a request/reply topic and
// blocks until the correlation-scoped reply arrives or ctx is done. name MUST
// be in topic.Events. On timeout it returns ErrNoResponder wrapping ctx.Err(),
// so errors.Is(err, context.DeadlineExceeded) still matches.
func (b *Bus) Request(ctx context.Context, topic RequestTopic, name, correlationID string, payload []byte) (*Event, error) {
	if !slices.Contains(topic.Events, name) {
		return nil, fmt.Errorf("%w: %q on request channel %q", ErrUnknownEvent, name, topic.Name)
	}
	if b.pubsub == nil {
		return nil, errNoPubSub
	}
	if correlationID == "" {
		correlationID = correlation.New()
	}

	// Subscribe to the correlation-scoped reply channel and wait for the
	// SUBSCRIBE ack before publishing, so the request cannot be answered
	// before we are listening. Scoping to the correlation id lets concurrent
	// callers share one request channel without stealing each other's replies.
	sub, err := b.pubsub.Subscribe(ctx, ReplyChannel(correlationID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = sub.Close() }()
	if err := sub.Receive(ctx); err != nil {
		return nil, err
	}

	encoded, err := MarshalEvent(&Event{
		Name:          name,
		Source:        b.cfg.Source,
		CorrelationId: correlationID,
		Timestamp:     timestamppb.New(b.now()),
		Payload:       payload,
	})
	if err != nil {
		return nil, err
	}
	if err := b.pubsub.Publish(ctx, topic.Name, encoded); err != nil {
		return nil, err
	}

	select {
	case replyPayload, ok := <-sub.Channel():
		if !ok {
			return nil, fmt.Errorf("%w: reply subscription closed for correlation_id=%s", ErrNoResponder, correlationID)
		}
		var reply Event
		if err := UnmarshalEvent(replyPayload, &reply); err != nil {
			return nil, err
		}
		return &reply, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrNoResponder, ctx.Err())
	}
}

// startPubSub opens one subscription per notify handler and per request/reply
// responder, waits for every SUBSCRIBE to be acknowledged, and starts a
// receive-loop goroutine for each. It is called from Run before the stream
// router starts, so Ready — gated on the router reporting itself running —
// cannot close until every subscription here is already live.
//
// A subscribe failure is a Run setup failure: startPubSub tears down whatever
// it opened, waits for those loops to exit, and returns the error so Run can
// return it and leave the Bus re-runnable.
func (b *Bus) startPubSub(ctx context.Context, wg *sync.WaitGroup) ([]PubSubSubscription, error) {
	var subs []PubSubSubscription

	open := func(channel string, loop func(PubSubSubscription)) error {
		sub, err := b.pubsub.Subscribe(ctx, channel)
		if err != nil {
			return err
		}
		if err := sub.Receive(ctx); err != nil {
			_ = sub.Close()
			return err
		}
		subs = append(subs, sub)
		wg.Add(1)
		go func() {
			defer wg.Done()
			loop(sub)
		}()
		return nil
	}

	fail := func(err error) ([]PubSubSubscription, error) {
		for _, sub := range subs {
			_ = sub.Close()
		}
		wg.Wait()
		return nil, err
	}

	for _, reg := range b.notifyRegs {
		if err := open(reg.topic.Name, func(sub PubSubSubscription) { b.consumeNotify(ctx, reg, sub) }); err != nil {
			return fail(fmt.Errorf("eventbus: subscribe to notify channel %s: %w", reg.topic.Name, err))
		}
	}
	for _, reg := range b.requestRegs {
		if err := open(reg.topic.Name, func(sub PubSubSubscription) { b.consumeRequest(ctx, reg, sub) }); err != nil {
			return fail(fmt.Errorf("eventbus: subscribe to request channel %s: %w", reg.topic.Name, err))
		}
	}
	return subs, nil
}

// consumeNotify is the per-handler receive loop for a notify topic. It hands
// each message's raw payload to the handler and returns when the subscription
// closes or ctx is cancelled.
func (b *Bus) consumeNotify(ctx context.Context, reg notifyRegistration, sub PubSubSubscription) {
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-messages:
			if !ok {
				return
			}
			reg.handler(ctx, payload)
		}
	}
}

// consumeRequest is the receive loop for a request/reply responder.
func (b *Bus) consumeRequest(ctx context.Context, reg requestRegistration, sub PubSubSubscription) {
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-messages:
			if !ok {
				return
			}
			b.dispatchRequest(ctx, reg, payload)
		}
	}
}

// dispatchRequest decodes one request, runs the handler, and publishes the
// reply envelope when the handler returned a payload. A decode failure or a
// handler error is logged at error level and sends no reply — the caller hits
// its ErrNoResponder timeout, matching the failure convention
// (docs/design/eventbus-bus-interface.md §3.5).
func (b *Bus) dispatchRequest(ctx context.Context, reg requestRegistration, payload []byte) {
	var request Event
	if err := UnmarshalEvent(payload, &request); err != nil {
		b.cfg.Logger.Error("eventbus: request responder could not decode request",
			"channel", reg.topic.Name,
			"error", err)
		return
	}

	replyPayload, err := reg.handler(ctx, &request)
	if err != nil {
		b.cfg.Logger.Error("eventbus: request responder failed",
			"channel", reg.topic.Name,
			"reply_name", reg.topic.ReplyName,
			"correlation_id", request.GetCorrelationId(),
			"error", err)
		return
	}
	if replyPayload == nil {
		return
	}

	encoded, err := MarshalEvent(&Event{
		Name:          reg.topic.ReplyName,
		Source:        b.cfg.Source,
		CorrelationId: request.GetCorrelationId(),
		Timestamp:     timestamppb.New(b.now()),
		Payload:       replyPayload,
	})
	if err != nil {
		b.cfg.Logger.Error("eventbus: request responder could not encode reply",
			"channel", reg.topic.Name,
			"reply_name", reg.topic.ReplyName,
			"correlation_id", request.GetCorrelationId(),
			"error", err)
		return
	}
	if err := b.pubsub.Publish(ctx, ReplyChannel(request.GetCorrelationId()), encoded); err != nil {
		b.cfg.Logger.Error("eventbus: request responder could not publish reply",
			"channel", reg.topic.Name,
			"reply_name", reg.topic.ReplyName,
			"correlation_id", request.GetCorrelationId(),
			"error", err)
	}
}
