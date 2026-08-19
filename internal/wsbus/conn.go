package wsbus

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"Metarr/internal/auth"
)

// conn is one client connection: the socket, the role its subscriptions are
// authorized against, an outbound queue, and the set of topics it is on.
//
// Writes all funnel through send and out via writeLoop, because a WebSocket
// permits only one concurrent writer and frames arrive here from several
// goroutines — one per producer, plus the reader answering pings.
type conn struct {
	ws   *websocket.Conn
	role auth.Role
	send chan []byte

	mu     sync.Mutex
	topics map[string]struct{}
}

// trySend queues a frame, dropping it if the connection is already backed up.
// Dropping is deliberate: every topic here carries a snapshot, so a frame
// that cannot be delivered now is superseded by the next one, and one slow
// client must not stall a producer serving everyone else.
func (c *conn) trySend(frame []byte) {
	select {
	case c.send <- frame:
	default:
	}
}

func (c *conn) addTopic(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.topics[name] = struct{}{}
}

func (c *conn) removeTopic(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.topics, name)
}

// topicNames returns a copy, so the caller can unsubscribe while iterating.
func (c *conn) topicNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	names := make([]string, 0, len(c.topics))
	for name := range c.topics {
		names = append(names, name)
	}
	return names
}

// writeLoop is the connection's sole writer. It returns when ctx is done,
// which the request handler triggers as soon as the read loop stops.
func (c *conn) writeLoop(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case frame := <-c.send:
			writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.ws.Write(writeCtx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				logger.Debug("websocket write failed", "error", err)
				return
			}

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.ws.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
