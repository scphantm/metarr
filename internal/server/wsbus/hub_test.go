package wsbus

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"Metarr/internal/server/auth"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// serveHub starts the hub behind a test server that injects role, standing in
// for the API key middleware.
func serveHub(t *testing.T, hub *Hub, role auth.Role) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeHTTP(w, r.WithContext(auth.WithRole(r.Context(), role)))
	}))
	t.Cleanup(server.Close)

	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func dial(t *testing.T, ctx context.Context, url string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })

	return conn
}

// readUntil returns the first frame whose type matches want, so a test is not
// tripped up by the acks and data frames interleaved around it.
func readUntil(t *testing.T, ctx context.Context, conn *websocket.Conn, want string) ServerMessage {
	t.Helper()

	for {
		var msg ServerMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			t.Fatalf("waiting for a %q frame: %v", want, err)
		}
		if msg.Type == want {
			return msg
		}
	}
}

func TestSubscribeStreamsTopicData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hub := New(ctx, discardLogger())
	hub.Register("stats.test", auth.GroupConfig, 10*time.Millisecond, func(context.Context) (any, error) {
		return map[string]int{"depth": 7}, nil
	})

	conn := dial(t, ctx, serveHub(t, hub, auth.RoleAdmin))
	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeSubscribe, Topic: "stats.test"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	data := readUntil(t, ctx, conn, TypeData)
	if data.Topic != "stats.test" {
		t.Errorf("topic = %q, want stats.test", data.Topic)
	}

	var payload map[string]int
	if err := json.Unmarshal(data.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["depth"] != 7 {
		t.Errorf("depth = %d, want 7", payload["depth"])
	}
}

func TestSubscribeRejectsUnauthorizedTopic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hub := New(ctx, discardLogger())
	// GroupConfig is admin-only, so a user-role caller must be refused.
	hub.Register("stats.test", auth.GroupConfig, time.Second, func(context.Context) (any, error) {
		return struct{}{}, nil
	})

	conn := dial(t, ctx, serveHub(t, hub, auth.RoleUser))
	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeSubscribe, Topic: "stats.test"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	msg := readUntil(t, ctx, conn, TypeError)
	if !strings.Contains(msg.Error, "not authorized") {
		t.Errorf("error = %q, want it to mention authorization", msg.Error)
	}

	// The connection must survive a refused subscribe.
	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypePing}); err != nil {
		t.Fatalf("connection died after a refused subscribe: %v", err)
	}
	readUntil(t, ctx, conn, TypePong)
}

func TestSubscribeRejectsUnknownTopic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hub := New(ctx, discardLogger())
	conn := dial(t, ctx, serveHub(t, hub, auth.RoleAdmin))

	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeSubscribe, Topic: "nope"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	msg := readUntil(t, ctx, conn, TypeError)
	if !strings.Contains(msg.Error, "unknown topic") {
		t.Errorf("error = %q, want it to mention the unknown topic", msg.Error)
	}
}

// The producer must run only while someone is listening: not before the first
// subscribe, and not after the last unsubscribe.
func TestProducerRunsOnlyWhileSubscribed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var collections atomic.Int64
	hub := New(ctx, discardLogger())
	hub.Register("stats.test", auth.GroupConfig, 5*time.Millisecond, func(context.Context) (any, error) {
		collections.Add(1)
		return struct{}{}, nil
	})

	url := serveHub(t, hub, auth.RoleAdmin)

	// Nothing should be collected while no one is subscribed.
	time.Sleep(30 * time.Millisecond)
	if got := collections.Load(); got != 0 {
		t.Fatalf("collected %d times before any subscriber", got)
	}

	conn := dial(t, ctx, url)
	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeSubscribe, Topic: "stats.test"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	readUntil(t, ctx, conn, TypeData)

	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeUnsubscribe, Topic: "stats.test"}); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	readUntil(t, ctx, conn, TypeAck)

	// Let the producer stop, then confirm it has.
	time.Sleep(20 * time.Millisecond)
	settled := collections.Load()
	time.Sleep(40 * time.Millisecond)

	if got := collections.Load(); got != settled {
		t.Errorf("collected %d more times after the last unsubscribe", got-settled)
	}
}

// Disconnecting must release subscriptions, or a producer would outlive every
// client that was watching it.
func TestDisconnectStopsProducer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var collections atomic.Int64
	hub := New(ctx, discardLogger())
	hub.Register("stats.test", auth.GroupConfig, 5*time.Millisecond, func(context.Context) (any, error) {
		collections.Add(1)
		return struct{}{}, nil
	})

	conn, _, err := websocket.Dial(ctx, serveHub(t, hub, auth.RoleAdmin), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeSubscribe, Topic: "stats.test"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	readUntil(t, ctx, conn, TypeData)

	conn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(30 * time.Millisecond)
	settled := collections.Load()
	time.Sleep(40 * time.Millisecond)

	if got := collections.Load(); got != settled {
		t.Errorf("collected %d more times after the client disconnected", got-settled)
	}
}

// Two subscribers share one producer, and it survives until the last leaves.
func TestProducerSharedAcrossSubscribers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var collections atomic.Int64
	hub := New(ctx, discardLogger())
	hub.Register("stats.test", auth.GroupConfig, 5*time.Millisecond, func(context.Context) (any, error) {
		collections.Add(1)
		return struct{}{}, nil
	})

	url := serveHub(t, hub, auth.RoleAdmin)

	first := dial(t, ctx, url)
	second := dial(t, ctx, url)

	for _, conn := range []*websocket.Conn{first, second} {
		if err := wsjson.Write(ctx, conn, ClientMessage{Type: TypeSubscribe, Topic: "stats.test"}); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		readUntil(t, ctx, conn, TypeData)
	}

	// One producer, not two: over a window of N intervals the count should
	// track N rather than 2N. Checking the topic's own state is the direct
	// assertion.
	hub.mu.Lock()
	subscribers := len(hub.topics["stats.test"].subscribers)
	hub.mu.Unlock()
	if subscribers != 2 {
		t.Fatalf("subscribers = %d, want 2", subscribers)
	}

	// The first leaving must not stop the second's stream.
	if err := wsjson.Write(ctx, first, ClientMessage{Type: TypeUnsubscribe, Topic: "stats.test"}); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	readUntil(t, ctx, first, TypeAck)

	before := collections.Load()
	readUntil(t, ctx, second, TypeData)
	if collections.Load() <= before {
		t.Error("producer stopped while a subscriber remained")
	}
}

func TestUnauthenticatedConnectionIsRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hub := New(ctx, discardLogger())

	// No role on the context, standing in for a request that bypassed the
	// API key middleware.
	server := httptest.NewServer(http.HandlerFunc(hub.ServeHTTP))
	defer server.Close()

	_, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err == nil {
		t.Fatal("dial succeeded without a role on the context")
	}
}
