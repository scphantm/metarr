package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// Handler implements slog.Handler. Handle does the minimum possible work
// before returning: build the record, write it to stdout, and offer it to
// the shipping buffer without waiting for room. Nothing here ever touches
// the network — that is Shipper's job, on its own goroutine — which is what
// makes Handle safe to call from a hot path without it becoming one.
type Handler struct {
	source string
	level  *slog.LevelVar
	stdout io.Writer

	buffer  chan<- []byte
	dropped *atomic.Int64

	// groups is the active WithGroup prefix chain, applied to attrs added
	// from here on — both future WithAttrs calls and this record's own
	// attrs. base holds attrs from earlier WithAttrs calls, already
	// flattened under whatever group prefix was active when each was added,
	// which is why they are stored resolved rather than as raw slog.Attr:
	// re-deriving their keys against the *current* groups would be wrong the
	// moment a WithGroup call happens in between.
	groups []string
	base   map[string]any
}

// Enabled reports whether level is at or above the handler's current
// threshold. The threshold is a shared *slog.LevelVar, so a level change
// made through Shipper.SetLevel takes effect on the next log call with no
// handler reconstruction needed.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle builds one Record, writes it to stdout, and offers it to the
// shipping buffer. A full buffer means the record is dropped and counted —
// never blocked on — which is the non-blocking guarantee this whole package
// exists to provide.
func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any, len(h.base)+record.NumAttrs())
	for key, value := range h.base {
		attrs[key] = value
	}
	record.Attrs(func(attr slog.Attr) bool {
		addFlatAttr(attrs, h.groups, attr)
		return true
	})
	if len(attrs) == 0 {
		attrs = nil
	}

	when := record.Time
	if when.IsZero() {
		when = time.Now()
	}

	encoded, err := marshalLogLine(
		when.UTC().Format(time.RFC3339Nano),
		record.Level.String(),
		record.Message,
		h.source,
		attrs,
	)
	if err != nil {
		// A record that fails to marshal is vanishingly unlikely — it means an
		// attribute value isn't JSON-encodable, which is already a slog misuse
		// — but this must not surface as an application error either way.
		return nil
	}

	// Best-effort, not error-checked: a failed stdout write is not something
	// logging should ever propagate back into application code.
	_, _ = h.stdout.Write(append(encoded, '\n'))

	select {
	case h.buffer <- encoded:
	default:
		h.dropped.Add(1)
	}
	return nil
}

// WithAttrs returns a new Handler with attrs flattened into it under the
// current group prefix. The receiver is left untouched, per the slog.Handler
// contract — callers may hold onto and keep using the original.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	next := h.clone()
	for _, attr := range attrs {
		addFlatAttr(next.base, next.groups, attr)
	}
	return next
}

// WithGroup returns a new Handler whose future attrs (from WithAttrs or the
// record itself) are prefixed with name. Attrs already flattened into base
// keep the prefix that was active when they were added.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	next := h.clone()
	next.groups = append(append([]string{}, h.groups...), name)
	return next
}

func (h *Handler) clone() *Handler {
	base := make(map[string]any, len(h.base))
	for key, value := range h.base {
		base[key] = value
	}

	return &Handler{
		source:  h.source,
		level:   h.level,
		stdout:  h.stdout,
		buffer:  h.buffer,
		dropped: h.dropped,
		groups:  h.groups,
		base:    base,
	}
}

// addFlatAttr resolves attr and stores it into dst under groups-prefixed
// dot-notation ("group.subgroup.key"), recursing into nested slog.Group
// values so no attribute is ever silently dropped for being structured.
func addFlatAttr(dst map[string]any, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	if attr.Value.Kind() == slog.KindGroup {
		// An anonymous group (empty key) merges its members at the current
		// level, exactly as slog's own handlers treat it.
		nested := groups
		if attr.Key != "" {
			nested = append(append([]string{}, groups...), attr.Key)
		}
		for _, inner := range attr.Value.Group() {
			addFlatAttr(dst, nested, inner)
		}
		return
	}

	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + attr.Key
	}
	dst[key] = attr.Value.Any()
}
