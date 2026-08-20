// Package logforward is the one hop that actually leaves Metarr's own
// infrastructure: the server forwards records it sees on the centralized
// logging channel to Fluent Bit's HTTP input, which is what ships them on to
// OpenObserve (or whatever it is configured to ship to instead).
//
// This exists only on the server, never the agent. Agents publish to Redis
// and nothing else — see CLAUDE.md's Logging section — so it is the server,
// which already needs broader network reach to serve HTTP and talk to Mongo,
// that takes on reaching Fluent Bit too. An agent never gains a network
// dependency beyond Redis because of this package.
package logforward

import (
	"bytes"
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// bufferSize bounds how many records may be queued for forwarding before
// Forward starts dropping them. The same non-blocking discipline as
// logging.Handler, for the same reason: a slow or unreachable Fluent Bit
// must never back up into the listener reading Redis.
const bufferSize = 2048

// requestTimeout bounds one POST to Fluent Bit, so a wedged connection can't
// pile up goroutines faster than they complete.
const requestTimeout = 5 * time.Second

// Forwarder relays raw log records — exactly as published to
// eventbus.LogChannel — to Fluent Bit's HTTP input.
type Forwarder struct {
	endpoint string
	client   *http.Client
	buffer   chan []byte
	dropped  atomic.Int64
	failed   atomic.Int64
}

// New returns a Forwarder that POSTs to endpoint (e.g.
// "http://fluent-bit:8888/app_logs") once Run is started.
func New(endpoint string) *Forwarder {
	return &Forwarder{
		endpoint: endpoint,
		client:   &http.Client{Timeout: requestTimeout},
		buffer:   make(chan []byte, bufferSize),
	}
}

// Forward offers one record for forwarding without waiting for room. A full
// buffer — Fluent Bit down, or genuinely can't keep up — means the record is
// dropped and counted rather than blocking the caller, which for this
// package is the listener's Redis read loop.
func (f *Forwarder) Forward(record []byte) {
	select {
	case f.buffer <- record:
	default:
		f.dropped.Add(1)
	}
}

// Run drains the buffer and POSTs each record to Fluent Bit until ctx is
// done. One record per request: Fluent Bit's http input takes a single JSON
// object per call, and centralized-logging volume here is already far below
// anything batching would meaningfully help.
func (f *Forwarder) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case record, ok := <-f.buffer:
			if !ok {
				return
			}
			f.post(ctx, record)
		}
	}
}

func (f *Forwarder) post(ctx context.Context, record []byte) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, f.endpoint, bytes.NewReader(record))
	if err != nil {
		f.failed.Add(1)
		return
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := f.client.Do(request)
	if err != nil {
		f.failed.Add(1)
		return
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode >= 300 {
		f.failed.Add(1)
	}
}

// Dropped and Failed report counters for the periodic self-diagnostic in
// cmd/metarr-server, matching the pattern logging.Shipper already uses.
func (f *Forwarder) Dropped() int64 { return f.dropped.Swap(0) }
func (f *Forwarder) Failed() int64  { return f.failed.Swap(0) }
