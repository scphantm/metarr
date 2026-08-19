package wsbus

import "encoding/json"

// Message types sent by the client.
const (
	// TypeSubscribe asks to start receiving a topic. The server answers with
	// an ack, or an error naming why not.
	TypeSubscribe = "subscribe"
	// TypeUnsubscribe stops delivery of a topic.
	TypeUnsubscribe = "unsubscribe"
	// TypePing asks for a pong. The server also sends protocol-level pings
	// on its own; this is for clients that want to measure the round trip.
	TypePing = "ping"
)

// Message types sent by the server.
const (
	// TypeData carries one topic payload.
	TypeData = "data"
	// TypeAck confirms a subscribe or unsubscribe.
	TypeAck = "ack"
	// TypeError reports a rejected request. It never closes the connection —
	// one bad subscribe should not cost a client its other topics.
	TypeError = "error"
	// TypePong answers TypePing.
	TypePong = "pong"
)

// ClientMessage is one frame from the browser.
type ClientMessage struct {
	Type  string `json:"type"`
	Topic string `json:"topic,omitempty"`
}

// ServerMessage is one frame to the browser.
type ServerMessage struct {
	Type    string          `json:"type"`
	Topic   string          `json:"topic,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}
