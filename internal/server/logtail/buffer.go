// Package logtail keeps the most recent log records the server has seen, for
// the live-tail pane on the System > Logging screen. It is fed from the same
// Redis Pub/Sub channel (eventbus.LogChannel) Fluent Bit ships from, so the
// pane shows exactly what the centralized pipeline is receiving — from every
// process, the server included, not only the one this buffer happens to run
// inside.
package logtail

import (
	"sync"

	"Metarr/internal/shared/logging"
)

// Buffer holds up to max recent records, oldest evicted first.
type Buffer struct {
	mu      sync.Mutex
	entries []*logging.Record
	max     int
}

// NewBuffer returns an empty Buffer holding at most max records.
func NewBuffer(max int) *Buffer {
	return &Buffer{max: max}
}

// Add decodes one raw record — exactly as published to eventbus.LogChannel —
// and appends it, evicting the oldest entry once the buffer is full. A
// record that fails to decode is dropped rather than breaking the tail for
// everything after it; the live view losing one malformed line is a far
// smaller problem than it wedging entirely.
func (b *Buffer) Add(raw []byte) {
	record, err := logging.UnmarshalRecord(raw)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = append(b.entries, record)
	if len(b.entries) > b.max {
		b.entries = b.entries[len(b.entries)-b.max:]
	}
}

// Recent returns a snapshot of the buffer's current contents, oldest first.
// The slice is freshly allocated, so later calls to Add do not disturb it;
// the records it points at are not copied and must be treated as read-only.
func (b *Buffer) Recent() []*logging.Record {
	b.mu.Lock()
	defer b.mu.Unlock()

	snapshot := make([]*logging.Record, len(b.entries))
	copy(snapshot, b.entries)
	return snapshot
}
