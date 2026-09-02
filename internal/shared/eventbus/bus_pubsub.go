package eventbus

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/timestamppb"

	"Metarr/internal/shared/correlation"
)

// This file is the Pub/Sub half of the Bus: fire-and-forget notifications
// (HandleNotify / Notify) and the one synchronous request/reply pattern
// (HandleRequest / Request). It holds cfg.Redis directly — there is no
// transport port for this half (docs/design/eventbus-bus-interface.md §4):
// miniredis in tests is the same go-redis adapter pointed at an in-process
// server, and a generic port would erase the SUBSCRIBE-ack-before-publish and
// correlation-scoped-reply semantics the code depends on.
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

// HandleNotify registers a fire-and-forget consumer for a KindNotify topic.
// Payload is opaque bytes — never decoded. Multiple handlers may share one
// topic; each opens its own subscription. All registration must precede Run.
func (b *Bus) HandleNotify(topic Topic, handler NotifyHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return ErrBusRunning
	}
	if topic.Kind != KindNotify {
		return fmt.Errorf("%w: HandleNotify needs a %s topic, got %q on %q", ErrWrongKind, KindNotify, topic.Kind, topic.Name)
	}
	if handler == nil {
		return fmt.Errorf("eventbus: HandleNotify on %q needs a handler", topic.Name)
	}
	if b.cfg.Redis == nil {
		return errors.New("eventbus: HandleNotify needs Config.Redis")
	}
	b.notifyRegs = append(b.notifyRegs, notifyRegistration{topic: topic, handler: handler})
	return nil
}

// HandleRequest registers the answering side for a KindRequestReply topic. The
// Bus decodes the request envelope, calls handler, and on a non-nil payload
// assembles the reply (Source = cfg.Source, correlation id copied from the
// request, Name = topic.ReplyName) and publishes it on
// ReplyChannel(correlation_id). A nil payload OR a handler error sends no
// reply, so the caller hits ErrNoResponder. Requires topic.ReplyName != "".
// One responder per topic; all registration must precede Run.
func (b *Bus) HandleRequest(topic Topic, handler RequestHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return ErrBusRunning
	}
	if topic.Kind != KindRequestReply {
		return fmt.Errorf("%w: HandleRequest needs a %s topic, got %q on %q", ErrWrongKind, KindRequestReply, topic.Kind, topic.Name)
	}
	if topic.ReplyName == "" {
		return fmt.Errorf("eventbus: request/reply topic %q has no ReplyName to stamp on the reply", topic.Name)
	}
	if handler == nil {
		return fmt.Errorf("eventbus: HandleRequest on %q needs a handler", topic.Name)
	}
	if b.cfg.Redis == nil {
		return errors.New("eventbus: HandleRequest needs Config.Redis")
	}
	if _, dup := b.requestRegs[topic.Name]; dup {
		return fmt.Errorf("eventbus: request/reply topic %q already has a responder", topic.Name)
	}
	b.requestRegs[topic.Name] = requestRegistration{topic: topic, handler: handler}
	return nil
}

// Notify publishes opaque bytes on a KindNotify topic. No envelope, no retry,
// at-most-once. Non-blocking.
func (b *Bus) Notify(ctx context.Context, topic Topic, payload []byte) error {
	if topic.Kind != KindNotify {
		return fmt.Errorf("%w: Notify needs a %s topic, got %q on %q", ErrWrongKind, KindNotify, topic.Kind, topic.Name)
	}
	if b.cfg.Redis == nil {
		return errors.New("eventbus: Notify needs Config.Redis")
	}
	return b.cfg.Redis.Publish(ctx, topic.Name, payload).Err()
}

// Request publishes a request envelope (Source = cfg.Source, the given name,
// the given correlationID — minted when "") on a KindRequestReply topic and
// blocks until the correlation-scoped reply arrives or ctx is done. name MUST
// be in topic.Events. On timeout it returns ErrNoResponder wrapping ctx.Err(),
// so errors.Is(err, context.DeadlineExceeded) still matches.
func (b *Bus) Request(ctx context.Context, topic Topic, name, correlationID string, payload []byte) (*Event, error) {
	if topic.Kind != KindRequestReply {
		return nil, fmt.Errorf("%w: Request needs a %s topic, got %q on %q", ErrWrongKind, KindRequestReply, topic.Kind, topic.Name)
	}
	if !slices.Contains(topic.Events, name) {
		return nil, fmt.Errorf("%w: %q on request channel %q", ErrUnknownEvent, name, topic.Name)
	}
	if b.cfg.Redis == nil {
		return nil, errors.New("eventbus: Request needs Config.Redis")
	}
	if correlationID == "" {
		correlationID = correlation.New()
	}

	// Subscribe to the correlation-scoped reply channel and wait for the
	// SUBSCRIBE ack before publishing, so the request cannot be answered
	// before we are listening. Scoping to the correlation id lets concurrent
	// callers share one request channel without stealing each other's replies.
	sub := b.cfg.Redis.Subscribe(ctx, ReplyChannel(correlationID))
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
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
	if err := b.cfg.Redis.Publish(ctx, topic.Name, encoded).Err(); err != nil {
		return nil, err
	}

	select {
	case replyMsg, ok := <-sub.Channel():
		if !ok {
			return nil, fmt.Errorf("%w: reply subscription closed for correlation_id=%s", ErrNoResponder, correlationID)
		}
		var reply Event
		if err := UnmarshalEvent([]byte(replyMsg.Payload), &reply); err != nil {
			return nil, err
		}
		return &reply, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", ErrNoResponder, ctx.Err())
	}
}

// startPubSub opens one Redis subscription per notify handler and per
// request/reply responder, waits for every SUBSCRIBE to be acknowledged, and
// starts a receive-loop goroutine for each. It is called from Run before the
// stream router starts, so Ready — gated on the router reporting itself
// running — cannot close until every subscription here is already live.
//
// A subscribe failure is a Run setup failure: startPubSub tears down whatever
// it opened, waits for those loops to exit, and returns the error so Run can
// return it and leave the Bus re-runnable.
func (b *Bus) startPubSub(ctx context.Context, wg *sync.WaitGroup) ([]*redis.PubSub, error) {
	var subs []*redis.PubSub

	open := func(channel string, loop func(*redis.PubSub)) error {
		sub := b.cfg.Redis.Subscribe(ctx, channel)
		if _, err := sub.Receive(ctx); err != nil {
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

	fail := func(err error) ([]*redis.PubSub, error) {
		for _, sub := range subs {
			_ = sub.Close()
		}
		wg.Wait()
		return nil, err
	}

	for _, reg := range b.notifyRegs {
		if err := open(reg.topic.Name, func(sub *redis.PubSub) { b.consumeNotify(ctx, reg, sub) }); err != nil {
			return fail(fmt.Errorf("eventbus: subscribe to notify channel %s: %w", reg.topic.Name, err))
		}
	}
	for _, reg := range b.requestRegs {
		if err := open(reg.topic.Name, func(sub *redis.PubSub) { b.consumeRequest(ctx, reg, sub) }); err != nil {
			return fail(fmt.Errorf("eventbus: subscribe to request channel %s: %w", reg.topic.Name, err))
		}
	}
	return subs, nil
}

// consumeNotify is the per-handler receive loop for a KindNotify topic. It
// hands each message's raw payload to the handler and returns when the
// subscription closes or ctx is cancelled.
func (b *Bus) consumeNotify(ctx context.Context, reg notifyRegistration, sub *redis.PubSub) {
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			reg.handler(ctx, []byte(msg.Payload))
		}
	}
}

// consumeRequest is the receive loop for a KindRequestReply responder.
func (b *Bus) consumeRequest(ctx context.Context, reg requestRegistration, sub *redis.PubSub) {
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			b.dispatchRequest(ctx, reg, msg.Payload)
		}
	}
}

// dispatchRequest decodes one request, runs the handler, and publishes the
// reply envelope when the handler returned a payload. A decode failure or a
// handler error is logged at error level and sends no reply — the caller hits
// its ErrNoResponder timeout, matching the failure convention
// (docs/design/eventbus-bus-interface.md §3.5).
func (b *Bus) dispatchRequest(ctx context.Context, reg requestRegistration, payload string) {
	var request Event
	if err := UnmarshalEvent([]byte(payload), &request); err != nil {
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
	if err := b.cfg.Redis.Publish(ctx, ReplyChannel(request.GetCorrelationId()), encoded).Err(); err != nil {
		b.cfg.Logger.Error("eventbus: request responder could not publish reply",
			"channel", reg.topic.Name,
			"reply_name", reg.topic.ReplyName,
			"correlation_id", request.GetCorrelationId(),
			"error", err)
	}
}
