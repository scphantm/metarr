package logforward

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Forward must never block, even with nothing draining the buffer — the
// worst case, and the one that matters: a caller of Forward is a Redis read
// loop, not something that can afford to wait on Fluent Bit.
func TestForwardNeverBlocksWithNothingDraining(t *testing.T) {
	forwarder := New("http://unused.invalid/app_logs")

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			forwarder.Forward([]byte(`{"message":"filling the buffer"}`))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Forward blocked instead of dropping once the buffer filled")
	}

	if got := forwarder.Dropped(); got <= 0 {
		t.Errorf("dropped count = %d, want > 0", got)
	}
}

// Run actually delivers records to the configured endpoint.
func TestRunPostsToTheEndpoint(t *testing.T) {
	var mu sync.Mutex
	var received [][]byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)

		mu.Lock()
		received = append(received, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	forwarder := New(server.URL + "/app_logs")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go forwarder.Run(ctx)

	forwarder.Forward([]byte(`{"message":"hello"}`))

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("forwarder never reached the test server")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// A Fluent Bit that is entirely unreachable must count failures, not hang or
// panic — the endpoint being down is a normal, expected state.
func TestRunCountsFailuresWhenUnreachable(t *testing.T) {
	forwarder := New("http://127.0.0.1:1/app_logs") // nothing listens on port 1

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go forwarder.Run(ctx)

	forwarder.Forward([]byte(`{"message":"nobody home"}`))

	deadline := time.After(3 * time.Second)
	for {
		if forwarder.Failed() > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("failed count never incremented for an unreachable endpoint")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
