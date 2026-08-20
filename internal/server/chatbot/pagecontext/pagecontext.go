// Package pagecontext is the generic mechanism by which whichever page the
// chat widget is open on contributes context to a conversation — the node
// catalog and current graph on the workflow page today, and (with zero
// changes to this file) whatever a future page needs later. A page is never
// hardcoded here: each one gets its own Assembler, keyed by the same
// pageKey the frontend's page-context registry uses.
package pagecontext

import (
	"context"
	"encoding/json"

	"Metarr/internal/server/chatbot/provider"
)

// Assembler turns one page's client-supplied context payload into a
// system-prompt contribution, plus whatever tools that page allows the
// model to call. Wiring propose_workflow_edit's Tools() to only the
// workflow assembler is what keeps "the AI can only propose edits on the
// workflow page" a structural fact rather than a prompt instruction that
// could be talked around.
type Assembler interface {
	// PageKey matches the frontend's useRegisterPageContext(pageKey, ...).
	PageKey() string
	// Assemble builds the system-prompt contribution and the record of what
	// was sent from clientPayload — the page-specific JSON blob the
	// frontend's collect() function produced. Anything the server already
	// knows in-process (e.g. the node catalog) is pulled in here directly,
	// not sent from the client.
	Assemble(ctx context.Context, clientPayload json.RawMessage) (Assembled, error)
	// Tools this page contributes to the model, if any.
	Tools() []provider.ToolSpec
}

// Assembled is one page's contribution to a single chat message.
type Assembled struct {
	// SystemText is appended to the conversation's system/developer message.
	SystemText string
	// Sent is exactly what the frontend's context-icon modal shows.
	Sent ContextSentRecord
}

// ContextSentRecord is persisted alongside the user message it was sent
// with, so the "what was sent" modal works for historical messages too.
type ContextSentRecord struct {
	PageKey string            `json:"page_key"`
	Items   []ContextSentItem `json:"items"`
}

// ContextSentItem is one labeled piece of context — e.g. "Node catalog" —
// with Detail carrying the actual trimmed payload for the modal's raw view.
type ContextSentItem struct {
	Label         string          `json:"label"`
	Description   string          `json:"description"`
	TokenEstimate int             `json:"token_estimate"`
	Detail        json.RawMessage `json:"detail"`
}

// Registry looks up the Assembler for a pageKey.
type Registry map[string]Assembler

// estimateTokens is a rough, provider-agnostic estimate (~4 characters per
// token, the commonly-cited rule of thumb for English text and JSON) — good
// enough for the context-sent modal's token-count display, not meant to
// match any one tokenizer exactly.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}
