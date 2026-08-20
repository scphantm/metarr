// Package summarize trims the node catalog and a workflow graph down to
// what an AI model actually needs to reason about a workflow — the biggest
// token-reduction lever the chatbot feature has, since the raw catalog and
// a saved graph both carry plenty of data (exec placement, canvas layout,
// long descriptions) that has nothing to do with proposing an edit.
package summarize

import (
	"Metarr/internal/shared/workflow"
)

// maxDescriptionLen bounds how much of a catalog description survives
// summarization — enough for the model to know what a node does, not so
// much that 34 entries' worth of prose dominates the context budget.
const maxDescriptionLen = 200

// CatalogEntry is one node type's trimmed shape: everything needed to
// reason about wiring a node correctly (ports, settings, what it does),
// nothing about where or how it executes (ExecSpec is dropped entirely —
// RunsOn/AgentSelector/Timeout/Retry are irrelevant to proposing a graph
// edit).
type CatalogEntry struct {
	Type        string       `json:"type"`
	TypeVersion string       `json:"type_version"`
	Name        string       `json:"name"`
	Category    string       `json:"category,omitempty"`
	Kind        string       `json:"kind,omitempty"`
	Description string       `json:"description,omitempty"`
	Control     ControlPorts `json:"control"`
	DataIn      []Socket     `json:"data_in,omitempty"`
	DataOut     []Socket     `json:"data_out,omitempty"`
	Settings    []Setting    `json:"settings,omitempty"`
}

// ControlPorts mirrors workflow.ControlPorts.
type ControlPorts struct {
	In    []string `json:"in,omitempty"`
	Out   []string `json:"out,omitempty"`
	Error bool     `json:"error,omitempty"`
}

// Socket is a trimmed workflow.Socket — name, type, and whether it's
// required. Label and Description are dropped: the model reasons about
// ports by name and type, not their display text.
type Socket struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

// Setting is a trimmed workflow.Setting — name, type, and default. Label,
// UI hints, and Description are dropped for the same reason as Socket's.
type Setting struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default any    `json:"default,omitempty"`
}

// Catalog trims every entry in catalog to its CatalogEntry shape, in
// catalog order.
func Catalog(catalog *workflow.Catalog) []CatalogEntry {
	entries := catalog.All()
	summarized := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		summarized = append(summarized, CatalogEntry{
			Type:        entry.Type,
			TypeVersion: entry.TypeVersion,
			Name:        entry.Name,
			Category:    entry.Category,
			Kind:        string(entry.Kind),
			Description: truncate(entry.Description, maxDescriptionLen),
			Control: ControlPorts{
				In:    entry.Control.In,
				Out:   entry.Control.Out,
				Error: entry.Control.Error,
			},
			DataIn:   summarizeSockets(entry.DataIn),
			DataOut:  summarizeSockets(entry.DataOut),
			Settings: summarizeSettings(entry.Settings),
		})
	}
	return summarized
}

func summarizeSockets(sockets []workflow.Socket) []Socket {
	if len(sockets) == 0 {
		return nil
	}
	summarized := make([]Socket, 0, len(sockets))
	for _, socket := range sockets {
		summarized = append(summarized, Socket{
			Name:     socket.Name,
			Type:     string(socket.Type),
			Required: socket.Required,
		})
	}
	return summarized
}

func summarizeSettings(settings []workflow.Setting) []Setting {
	if len(settings) == 0 {
		return nil
	}
	summarized := make([]Setting, 0, len(settings))
	for _, setting := range settings {
		summarized = append(summarized, Setting{
			Name:    setting.Name,
			Type:    string(setting.Type),
			Default: setting.Default,
		})
	}
	return summarized
}

// truncate cuts s to at most n runes, marking the cut with an ellipsis so
// the model can tell the description was shortened rather than reading as
// a complete (and confusingly abrupt) sentence.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
