// Package provider unifies Claude, OpenAI, Gemini, and an MCP-connected
// model behind one streaming completion interface, so the rest of the
// chatbot feature never branches on which provider is active — that
// happens once, in Select.
package provider

import (
	"context"
	"encoding/json"
)

// Message is one turn in the conversation sent to the model.
type Message struct {
	Role    string // "user" | "assistant"
	Content string
}

// ToolSpec is a function the model may call, in the JSON-Schema shape every
// provider's tool-calling feature accepts. Only populated when the active
// page context contributes one (e.g. the workflow page's
// propose_workflow_edit — see internal/server/chatbot/pagecontext).
type ToolSpec struct {
	Name        string
	Description string
	JSONSchema  json.RawMessage
}

// CompletionRequest is one turn of conversation plus whatever page context
// and tools apply to it.
type CompletionRequest struct {
	Messages []Message
	System   string // assembled page context, injected as the system/developer message
	Tools    []ToolSpec
}

// ToolCall is a model-requested invocation of one of CompletionRequest's
// Tools. Tagged (unlike Message/ToolSpec) because, unlike those two, this
// type crosses the wire — nested inside Delta.MarshalJSON's output and
// stored as the JSON-encoded ChatMessage.ToolCall field.
type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Delta is one increment of a streamed reply. Exactly one of Text, ToolCall,
// or (Done && Err) is meaningful per call to emit.
type Delta struct {
	Text     string
	ToolCall *ToolCall
	Done     bool
	Err      error
}

// MarshalJSON renders Err as a plain string. Go's encoding/json has no
// useful default for an `error` field — most error implementations are
// structs with unexported fields — so without this, a final Delta whose
// only content is Err would silently encode as {} on the wire and a client
// would see nothing went wrong.
func (d Delta) MarshalJSON() ([]byte, error) {
	wire := struct {
		Text     string    `json:"text,omitempty"`
		ToolCall *ToolCall `json:"tool_call,omitempty"`
		Done     bool      `json:"done,omitempty"`
		Err      string    `json:"error,omitempty"`
	}{
		Text:     d.Text,
		ToolCall: d.ToolCall,
		Done:     d.Done,
	}
	if d.Err != nil {
		wire.Err = d.Err.Error()
	}
	return json.Marshal(wire)
}

// Provider streams a completion for req, calling emit for every incremental
// chunk and ending with a Delta{Done: true} (Err set only if generation
// failed). Implementations must call emit from the goroutine Stream runs on
// — callers may write emitted deltas straight to a network connection and
// rely on that ordering.
type Provider interface {
	Stream(ctx context.Context, req CompletionRequest, emit func(Delta)) error
}
