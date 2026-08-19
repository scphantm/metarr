// Package wsbus is the server→client streaming layer: one WebSocket
// connection per browser tab, carrying any number of named topics that the
// client subscribes to and unsubscribes from over that single connection.
//
// A topic is registered once, with the role group required to read it, how
// often to produce, and what to produce. The hub owns the lifecycle from
// there: a topic's producer starts when its first subscriber arrives and is
// cancelled when the last one leaves, so a server nobody is watching does no
// work. Each tick collects once and fans the same encoded bytes out to every
// subscriber, so cost is per topic rather than per connection.
package wsbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"Metarr/internal/auth"
)

const (
	// sendBuffer is how many frames may be queued for one connection before
	// the hub starts dropping them. Topics here are snapshots — a frame that
	// cannot be delivered promptly is superseded by the next one — so a slow
	// client should lose frames rather than stall the producer for everyone.
	sendBuffer = 16

	// pingInterval keeps idle connections alive through proxies that drop
	// silent ones.
	pingInterval = 30 * time.Second

	// writeTimeout bounds a single frame write, so one wedged socket cannot
	// hold its writer goroutine forever.
	writeTimeout = 10 * time.Second
)

// Hub routes topics to connections.
type Hub struct {
	logger *slog.Logger

	// baseCtx bounds every producer goroutine to the server's lifetime,
	// independent of the request that happened to start one.
	baseCtx context.Context

	mu     sync.Mutex
	topics map[string]*topic
}

type topic struct {
	name     string
	group    auth.Group
	interval time.Duration
	collect  func(context.Context) (any, error)

	// Both guarded by Hub.mu.
	subscribers map[*conn]struct{}
	cancel      context.CancelFunc
}

// New returns a hub whose producers stop when ctx is cancelled.
func New(ctx context.Context, logger *slog.Logger) *Hub {
	return &Hub{
		logger:  logger,
		baseCtx: ctx,
		topics:  make(map[string]*topic),
	}
}

// Register declares a topic. group is the role group a caller must be
// authorized for to subscribe; interval is how often collect is called while
// anyone is listening. Registering the same name twice overwrites the first,
// and is a programming error rather than a runtime condition.
func (h *Hub) Register(name string, group auth.Group, interval time.Duration, collect func(context.Context) (any, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.topics[name] = &topic{
		name:        name,
		group:       group,
		interval:    interval,
		collect:     collect,
		subscribers: make(map[*conn]struct{}),
	}
}

// ServeHTTP upgrades the request and serves that connection until it closes.
// It expects to run behind the API key middleware, which resolves the role
// this connection's subscriptions are authorized against.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	role, ok := auth.RoleFromContext(r.Context())
	if !ok {
		http.Error(w, "missing or invalid API key", http.StatusUnauthorized)
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Origin checking defends against a hostile page riding an ambient
		// credential — a cookie the browser attaches on its own. This API
		// has none: the key is held in sessionStorage and passed explicitly,
		// so a foreign origin has nothing to ride and the check buys no
		// safety. It would, meanwhile, reject the dev setup outright, where
		// Vite proxies :5173 to the API on :8080 and the two necessarily
		// disagree.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		h.logger.Warn("websocket upgrade failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	connection := &conn{
		ws:     ws,
		role:   role,
		send:   make(chan []byte, sendBuffer),
		topics: make(map[string]struct{}),
	}

	go connection.writeLoop(ctx, h.logger)

	err = h.readLoop(ctx, connection)

	// Drop every subscription before closing, so a disconnect stops the
	// producers this connection was the last subscriber of.
	h.unsubscribeAll(connection)

	switch {
	case err == nil, errors.Is(err, context.Canceled):
		ws.Close(websocket.StatusNormalClosure, "")
	case websocket.CloseStatus(err) != -1:
		// The client closed; that is not an error worth logging.
	default:
		h.logger.Debug("websocket closed", "error", err)
		ws.Close(websocket.StatusInternalError, "read failed")
	}
}

func (h *Hub) readLoop(ctx context.Context, c *conn) error {
	for {
		var msg ClientMessage
		if err := wsjson.Read(ctx, c.ws, &msg); err != nil {
			return err
		}

		switch msg.Type {
		case TypeSubscribe:
			if err := h.subscribe(c, msg.Topic); err != nil {
				c.trySend(encode(ServerMessage{
					Type:  TypeError,
					Topic: msg.Topic,
					Error: err.Error(),
				}))
				continue
			}
			c.trySend(encode(ServerMessage{Type: TypeAck, Topic: msg.Topic}))

		case TypeUnsubscribe:
			h.unsubscribe(c, msg.Topic)
			c.trySend(encode(ServerMessage{Type: TypeAck, Topic: msg.Topic}))

		case TypePing:
			c.trySend(encode(ServerMessage{Type: TypePong}))

		default:
			c.trySend(encode(ServerMessage{
				Type:  TypeError,
				Error: fmt.Sprintf("unknown message type %q", msg.Type),
			}))
		}
	}
}

func (h *Hub) subscribe(c *conn, name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	registered, ok := h.topics[name]
	if !ok {
		return fmt.Errorf("unknown topic %q", name)
	}

	// A topic stream is a read, so it is authorized as one: this lets the
	// read-only role subscribe to topics its group covers, matching how the
	// REST routes treat it.
	if !auth.Authorized(c.role, registered.group, http.MethodGet) {
		return fmt.Errorf("not authorized for topic %q", name)
	}

	if _, already := registered.subscribers[c]; already {
		return nil
	}

	c.addTopic(name)
	registered.subscribers[c] = struct{}{}

	if len(registered.subscribers) == 1 {
		h.startProducerLocked(registered)
	}
	return nil
}

func (h *Hub) unsubscribe(c *conn, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.unsubscribeLocked(c, name)
}

func (h *Hub) unsubscribeAll(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, name := range c.topicNames() {
		h.unsubscribeLocked(c, name)
	}
}

func (h *Hub) unsubscribeLocked(c *conn, name string) {
	registered, ok := h.topics[name]
	if !ok {
		return
	}

	delete(registered.subscribers, c)
	c.removeTopic(name)

	if len(registered.subscribers) == 0 && registered.cancel != nil {
		registered.cancel()
		registered.cancel = nil
	}
}

// startProducerLocked must be called with h.mu held.
func (h *Hub) startProducerLocked(t *topic) {
	ctx, cancel := context.WithCancel(h.baseCtx)
	t.cancel = cancel

	go func() {
		ticker := time.NewTicker(t.interval)
		defer ticker.Stop()

		// Produce once straight away so the first subscriber sees data
		// without waiting out a full interval.
		h.produce(ctx, t)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.produce(ctx, t)
			}
		}
	}()
}

// produce collects one value and fans it out. The payload is encoded once
// per tick regardless of how many connections receive it.
func (h *Hub) produce(ctx context.Context, t *topic) {
	value, err := t.collect(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		h.logger.Error("topic collection failed", "topic", t.name, "error", err)
		h.broadcast(t, encode(ServerMessage{
			Type:  TypeError,
			Topic: t.name,
			Error: err.Error(),
		}))
		return
	}

	payload, err := json.Marshal(value)
	if err != nil {
		h.logger.Error("topic payload could not be encoded", "topic", t.name, "error", err)
		return
	}

	h.broadcast(t, encode(ServerMessage{
		Type:    TypeData,
		Topic:   t.name,
		Payload: payload,
	}))
}

func (h *Hub) broadcast(t *topic, frame []byte) {
	h.mu.Lock()
	targets := make([]*conn, 0, len(t.subscribers))
	for c := range t.subscribers {
		targets = append(targets, c)
	}
	h.mu.Unlock()

	for _, c := range targets {
		c.trySend(frame)
	}
}

func encode(msg ServerMessage) []byte {
	frame, err := json.Marshal(msg)
	if err != nil {
		// ServerMessage is all encodable types apart from Payload, which is
		// already valid JSON by the time it lands here.
		return []byte(`{"type":"error","error":"internal encoding failure"}`)
	}
	return frame
}
