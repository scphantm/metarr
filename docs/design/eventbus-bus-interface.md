# `eventbus.Bus` — interface design

Implementation design for **ADR-0008** (the bus contract is language-agnostic, Go
holds the reference implementation). This is the Go reference adapter's shape. The
normative cross-language contract — envelope, names, reply-channel formula, entry
bytes — belongs in `documentation/modules/design/pages/event_bus.adoc` (to be
written from this plus the wire details); where this doc and that page disagree,
the page wins.

Chosen from a design-it-twice pass over four candidates: a **ports-and-adapters**
spine with **one unified `Topic` table** covering streams and channels alike.

> **Amended 2026-09-02**, after the first review of the built `Bus`: the verbs
> now take **kind-typed topic handles** (`StreamTopic` / `NotifyTopic` /
> `RequestTopic`, §2), retiring the runtime `ErrWrongKind`; and the **Pub/Sub
> half gained a port** (`PubSubTransport`, §4.2) so the whole `Bus` is testable
> with no Redis, not just its durable-stream half. Both were "refusals" in the
> original design; the notes at §2 and §4.2 record why the trade-off changed.
> The unified `Topic` row is unchanged — it stays the plain-data enumeration
> shape; the handles wrap it only where a Go call site drives a verb.

Related: ADR-0006 (one bus contract over Redis + the 2026-09-01 minimal-marshaller
amendment), ADR-0009 (no credential in a bus payload), ADR-0007 (expected-vs-actual
topology), `CONTEXT.md` → Event bus.

---

## 1. What it replaces

| Today | Becomes |
|---|---|
| `StreamBus`, `PubSubBus`, `Router`, `PubSubRouter` — four types a caller assembles | one `*Bus` |
| `NewEvent(source, name, correlationID, payload)` hand-called at every publish site | envelope assembled by the `Bus` from the `Topic` row + call args |
| `StreamTopic` + bare Pub/Sub channel strings + `KnownPubSubChannels()` + `AgentPubSubChannels(slug)` | one `Topic` value, one `Topics()` table, `Kind`-tagged |
| server builds `StreamBus` + `appConfigStore` **twice** (once on `DefaultBusPolicy`, once on live policy) | built once; policy is a late-bound `func() BusPolicy` |
| two routers, each `Run`/`Running`/`Close`, driven by a copy-pasted `startRouter` closure in both `main.go`s | one `Run(ctx)`, one `Ready()`, one `Close()` |
| every stream listener re-implements `switch event.Name { … default: warn }` | per-`(topic, name)` dispatch; the unknown-name default lives once inside the `Bus` |

Retained verbatim (contract / name table, not indirection): `Event` (=
`metarrv1.EventEnvelope`), `SourceServer`, `AgentSource`, `SlugFromAgentSource`,
`ReplyChannel`, `DefaultBusPolicy`, `BusPolicyFromConfig`, `DefaultRetentionPolicy`,
`DefaultRetryPolicy`, `StreamIDForTime`, `TimeFromStreamID`, `ErrNoResponder`.

---

## 2. `Topic` — the one table

`StreamTopic` becomes `Topic` with a `Kind`. Every durable stream **and** every
fixed Pub/Sub channel is a row. Plain data, so a participant in another language
can carry the same table.

```go
type TopicKind string

const (
	KindStream       TopicKind = "stream"        // durable Redis Stream; Watermill Router; at-least-once + bounded retry
	KindNotify       TopicKind = "notify"        // Pub/Sub; fire-and-forget; at-most-once; payload is opaque bytes
	KindRequestReply TopicKind = "request_reply" // Pub/Sub request channel + correlation-scoped reply
)

type Topic struct {
	Name string // literal stream/channel name, or a glob when Pattern is set

	Kind    TopicKind
	Pattern bool // Name is a glob (KindStream only); concrete rows come from DiscoverTopics

	Group    string // consumer group — KindStream only; "" when nothing consumes it
	Consumed bool   // a listener is registered / an identity is expected to be attached (ADR-0007)

	// Events are the envelope Name discriminators legal on this topic. LOAD-BEARING:
	//   - KindStream / KindRequestReply: a publish or a handler registration naming
	//     an event not in this list is rejected (ErrUnknownEvent).
	//   - KindNotify: advisory only. A notify payload need not be an envelope
	//     (LogTopic carries raw slog records), so nothing is decoded or validated.
	Events []string

	// ReplyName is the Name stamped on the reply envelope. KindRequestReply only;
	// required for that kind, "" otherwise.
	ReplyName string
}
```

`AgentScanResultTopic().Events` widens from the single informational `agent.scan_result`
to the real `["agent.scan_result", "agent.scan_complete", "agent.scan_failed"]` — the
field is now the dispatch table, not a comment.

### Typed topic handles (amended 2026-09-02)

The original design had one `Kind`-tagged `Topic` and every `Bus` verb taking a
bare `Topic`, with a wrong-`Kind` call — `Publish` on a notify row, say — caught
at run time as `ErrWrongKind`. That check turned out to be pure friction: it
fires on the first test run, and six near-identical guard blocks plus six
`…RejectsWrongKind` tests existed only to police it.

So each verb now takes a one-field wrapper that carries the kind in the Go type:

```go
type StreamTopic  struct{ Topic } // HandleStream, Publish
type NotifyTopic   struct{ Topic } // HandleNotify, Notify
type RequestTopic  struct{ Topic } // HandleRequest, Request
```

The topic constructors return the matching wrapper (`SystemConfigUpdateTopic()
StreamTopic`, `HeartbeatTopic() RequestTopic`, …). A verb handed the wrong one
no longer compiles; `ErrWrongKind` is deleted.

`Topic` is unchanged and stays the enumeration row: `Topics()`, `AgentTopics()`,
`DiscoverStreamTopics()` still return `[]Topic` (built by unwrapping the
constructors' `.Topic`), so ADR-0007's topology derivation and the stats
dashboard are untouched — plain data for a participant in another language, a
typed handle only where a Go call site drives a verb. A hand-assembled
`StreamTopic{Topic{Name: "…"}}` with a bogus name is still caught by
`streamTopicPublishable` inside `Publish` (`ErrNotPublishable`).

### Constructors (`topics.go`)

```go
// KindStream
func SystemConfigUpdateTopic() Topic
func AgentScanResultTopic() Topic
func AgentNodeResultTopic() Topic          // reserved: Group "", Consumed false, no handler yet
func AgentCommandTopic(slug string) Topic  // Events: ["agent.scan"]

// KindNotify
func LogTopic() Topic                          // Events: nil — raw records, never an envelope
func AgentConfigChangedTopic(slug string) Topic // Events: nil — empty payload

// KindRequestReply
func HeartbeatTopic() Topic                 // Events: ["heartbeat.request"],  ReplyName: "heartbeat.reply"
func AgentNFOReadTopic(slug string) Topic   // Events: ["agent.nfo_read"],     ReplyName: "agent.nfo_read_reply"

// the table
func Topics() []Topic                                                  // static rows + the one KindStream pattern row
func DiscoverTopics(ctx context.Context, c redis.UniversalClient) ([]Topic, error) // expands KindStream patterns against live Redis
```

ADR-0007's expected-vs-actual derivation reads `Topics()` (plus per-agent expansion
from the agent registry) instead of `StreamTopics()` + `KnownPubSubChannels()` +
`AgentPubSubChannels()`. One list, filtered by `Kind` where a caller needs only
streams (retention sweep, publish cap) or only channels.

---

## 3. The `Bus`

### 3.1 Construction — once, before bootstrap

```go
type Config struct {
	// Redis is the shared client for the Pub/Sub half and the retention sweep's
	// admin ops (XTRIM, SCAN). The Bus NEVER calls Close on it (constraint: shared,
	// not owned). May be nil when only KindStream is used with ChannelStreamTransport
	// (a pure-stream unit test).
	Redis redis.UniversalClient

	// Source is stamped on every envelope this process publishes and every reply it
	// sends: SourceServer, or AgentSource(slug). It also DERIVES the durable-stream
	// consumer identity (SourceServer → ConsumerName; AgentSource(slug) → slug). No
	// call site ever names either. Open string — the contract does not enumerate
	// valid Source values (ADR-0008).
	Source string

	// Streams is the durable-stream transport port (§4). Production:
	// RedisStreamTransport(client, adapter). Tests: ChannelStreamTransport().
	Streams StreamTransport

	// PubSub is the Pub/Sub transport (§4, amended 2026-09-02). Production
	// leaves it nil and New defaults it to RedisPubSub(Redis), so both
	// binaries stay on Redis with no wiring change. Tests pass
	// InMemoryPubSub() to run notify + request/reply with no broker. Nil PubSub
	// AND nil Redis → the Pub/Sub verbs error, as the old missing-Redis case did.
	PubSub PubSubTransport

	// Policy is the late-bound tuning provider. Never nil. Read once in Run for the
	// retry stack + publish MAXLEN, and once per iteration by the retention sweep.
	// Before the event_bus config section is bootstrapped it returns
	// DefaultBusPolicy(); after, the server's closure returns the live values.
	Policy func() BusPolicy

	Logger *slog.Logger

	// RetentionSweep runs the age-based XTRIM sweep inside Run. Server: true.
	// Agent: false (it has no canonical view to trim).
	RetentionSweep bool

	// Now stamps envelope timestamps and the retention cutoff. Zero value →
	// func() time.Time { return time.Now().UTC() }.
	Now func() time.Time
}

func New(cfg Config) (*Bus, error)
```

### 3.2 Registration — all before `Run`

```go
type StreamHandler  func(ctx context.Context, event *Event) error
type RequestHandler func(ctx context.Context, request *Event) (replyPayload []byte, err error)
type NotifyHandler  func(ctx context.Context, payload []byte)

// HandleStream registers the sole consumer of a KindStream topic. The map key is
// the envelope Name; dispatch is per (topic, Name). Every key MUST be in
// topic.Events (else ErrUnknownEvent). An event whose Name is on the stream but
// not in the map hits the unknown-name default — logged once with stream + name,
// then acked — which lives in exactly one place inside the Bus. Rejects a pattern
// topic and a topic with Group == "".
func (b *Bus) HandleStream(topic StreamTopic, handlers map[string]StreamHandler) error

// HandleRequest registers the answering side for a KindRequestReply topic. The Bus
// decodes the request envelope, calls handler, and on a non-nil payload assembles
// the reply (Source = cfg.Source, correlation_id copied from the request, Name =
// topic.ReplyName) and publishes it on ReplyChannel(correlation_id). A nil payload
// OR a handler error sends no reply, so the caller hits ErrNoResponder. Requires
// topic.ReplyName != "".
func (b *Bus) HandleRequest(topic RequestTopic, handler RequestHandler) error

// HandleNotify registers a fire-and-forget consumer for a KindNotify topic. Payload
// is opaque bytes — never decoded. Multiple handlers may share one topic; each
// opens its own subscription.
func (b *Bus) HandleNotify(topic NotifyTopic, handler NotifyHandler) error
```

Registration after `Run` returns `ErrBusRunning`.

### 3.3 Publish / request — one verb per `Kind`

```go
// Publish appends one event to a KindStream topic. The caller supplies name,
// correlationID, payload. The Bus assembles the envelope: Source = cfg.Source
// (unforgeable at the call site), Name = name (MUST be in topic.Events, else
// ErrUnknownEvent), Timestamp = cfg.Now(). It applies MAXLEN ~ Policy().Retention.MaxLen.
// Rejects a pattern topic or a name the table does not resolve (ErrNotPublishable).
// Non-blocking. correlationID is passed explicitly — the Bus does NOT read it from
// ctx (the wire contract has no ctx; a non-Go publisher sets correlation_id itself).
func (b *Bus) Publish(ctx context.Context, topic StreamTopic, name, correlationID string, payload []byte) error

// Notify publishes opaque bytes on a KindNotify topic. No envelope, no retry.
func (b *Bus) Notify(ctx context.Context, topic NotifyTopic, payload []byte) error

// Request publishes a request envelope (Source = cfg.Source, the given name, the
// given correlationID — minted if "") on a KindRequestReply topic and blocks until
// the correlation-scoped reply arrives or ctx is done. On timeout it returns
// ErrNoResponder wrapping ctx.Err(), so errors.Is(err, context.DeadlineExceeded)
// still matches. name MUST be in topic.Events.
func (b *Bus) Request(ctx context.Context, topic RequestTopic, name, correlationID string, payload []byte) (*Event, error)
```

The verbs take `StreamTopic` / `NotifyTopic` / `RequestTopic` (§2), so a
wrong-kind call does not compile — there is no `ErrWrongKind`.

### 3.4 Lifecycle

```go
// Run starts BOTH transports — the Watermill durable-stream router and the Pub/Sub
// receive loops — plus the retention sweep when cfg.RetentionSweep, in one errgroup,
// and blocks until ctx is cancelled or a transport exits. It reads Policy() once
// for the retry stack + MAXLEN. All registration must precede it. At most one call.
func (b *Bus) Run(ctx context.Context) error

// Ready is closed once every stream handler is live AND every Pub/Sub SUBSCRIBE is
// acknowledged. A warm-up publisher waits on it.
func (b *Bus) Ready() <-chan struct{}

// Close tears down the Watermill router, the Pub/Sub subscriptions, and the stream
// transport's publisher. It NEVER closes cfg.Redis.
func (b *Bus) Close() error
```

### 3.5 Invariants, ordering, errors

- **`Source` binds once**, from `cfg.Source`. No `source` parameter exists anywhere.
  A publish naming the wrong process is unrepresentable.
- **`Name` binds from the row.** `Publish`/`Request`/`HandleStream` validate `name`
  against `topic.Events` before any Redis call. `HandleRequest` takes its reply name
  from `topic.ReplyName`.
- **Consumer identity** derives from `cfg.Source`; never a parameter.
- **`correlationID` is explicit** on `Publish`/`Request`. A caller holding it in ctx
  passes `correlation.FromContext(ctx)` itself. `Request` mints one when `""`.
- **Streams are at-least-once**, no cross-handler ordering, one server-side consumer
  (ADR-0002).
- **Failure convention (normative, unchanged — ADR-0006):** a `StreamHandler`
  returns an error **only** when the message could not be processed at all
  (undecodable payload, datastore unreachable) → retried per `Policy().Retry` →
  `dropAfterRetry` logs at error with the message id and acks (dropped); no parking
  stream. Work that ran and produced a business failure publishes a failure result
  event and returns `nil`, and never touches the retry path. A `RequestHandler`
  returning `(nil, x)` or an error sends no reply → the caller hits `ErrNoResponder`.
- **Errors** (all `errors.Is`-matchable): `ErrNoResponder`, `ErrBusRunning`,
  `ErrUnknownEvent`, `ErrNotPublishable`.
- **ADR-0009:** no envelope payload — stream, notify or reply — may carry a
  credential. Not enforced by the type system; a review check per new payload.

---

## 4. The ports

> **Amended 2026-09-02.** This section originally described **one** port,
> `StreamTransport`, and listed `PubSubTransport` under "seams deliberately not
> taken". The Pub/Sub seam was subsequently taken — see §4.2. Everything about
> `StreamTransport` below is unchanged.

### 4.1 `StreamTransport` — the durable-stream port

```go
// StreamPublisher appends one durable-stream entry carrying `envelope` as its
// payload. The adapter owns the entry framing (the {"payload": <envelope JSON>}
// minimal marshaller — ADR-0006 2026-09-01) and the approximate MAXLEN cap. The
// Bus owns envelope assembly and protojson.
type StreamPublisher interface {
	Publish(ctx context.Context, stream string, envelope []byte, approxMaxLen int64) error
	Close() error
}

// StreamTransport is the durable-stream transport. Subscriber returns a Watermill
// subscriber whose delivered message.Payload is the envelope JSON (the Redis
// adapter's marshaller strips the {"payload": …} wrapper; the channel adapter
// passes through). The Watermill type in the signature is a deliberate leak,
// unchanged from today's SubscriberFactory (ADR-0006 accepts it).
type StreamTransport interface {
	Publisher() (StreamPublisher, error)
	Subscriber(group, consumer string) (message.Subscriber, error)
}

func RedisStreamTransport(client redis.UniversalClient, logger watermill.LoggerAdapter) StreamTransport
func ChannelStreamTransport() StreamTransport // one gochannel.GoChannel as publisher + every subscriber
```

| Adapter | Role | Used by |
|---|---|---|
| `RedisStreamTransport` | production — watermill-redisstream publisher + per-group subscribers over the shared client; owns the minimal marshaller, consumer-group creation, `XAUTOCLAIM` reclaim | `cmd/metarr-server/main.go`, `cmd/metarr-agent/main.go` |
| `ChannelStreamTransport` | tests — proves handler logic + middleware + `(topic, name)` dispatch with no Redis; `group`/`consumer` ignored | `internal/shared/eventbus/*_test.go`, `internal/server/listeners/*_test.go`, `internal/agent/runtime/*_test.go` |

### 4.2 `PubSubTransport` — the Pub/Sub port (added 2026-09-02)

The original design refused this seam: one real implementation (go-redis
`UniversalClient`), miniredis being the *same* adapter at an in-process server
rather than a second one, and the worry that a generic port would erase the
SUBSCRIBE-ack-before-publish and correlation-scoped-reply semantics.

Two of those held; one did not. `ChannelStreamTransport` had already made the
durable-stream half runnable with no Redis, and the Pub/Sub half was the only
part of the `Bus` a test could not reach without a Redis-shaped thing — so
`bus_pubsub_test.go` and every `listeners` / `runtime` test touching notify or
request/reply carried a miniredis dependency the stream half had shed. That is
real, recurring friction, and it is exactly the split ADR-0008's deferred
conformance harness has to cross. The ack-and-reply worry is handled by keeping
that logic on the `Bus` side of the seam: the port is two methods, `Subscribe`
(returning a subscription whose `Receive` is the ack) and `Publish`.

```go
type PubSubTransport interface {
	Subscribe(ctx context.Context, channel string) (PubSubSubscription, error)
	Publish(ctx context.Context, channel string, payload []byte) error
}
type PubSubSubscription interface {
	Receive(ctx context.Context) error // blocks until SUBSCRIBE is acked
	Channel() <-chan []byte
	Close() error
}

func RedisPubSub(client redis.UniversalClient) PubSubTransport
func InMemoryPubSub() PubSubTransport // in-process broker for tests
```

| Adapter | Role | Used by |
|---|---|---|
| `RedisPubSub` | production — near-passthrough over the shared client; owns nothing, closes nothing. **Never named at a call site**: `Config.PubSub` is left nil and `New` defaults to it, so both binaries stay on Redis with no wiring change. | `eventbus.New` default |
| `InMemoryPubSub` | tests — in-process broker with the two properties the `Bus` needs (a subscription live the instant `Subscribe` returns; `Publish` fans out to every subscriber). Retires miniredis from `bus_pubsub_test.go`. | `internal/shared/eventbus/*_test.go`, `internal/server/listeners/*_test.go` |

It parallels `StreamTransport` exactly, including being an exported port whose
"don't use this adapter in production" is convention, not structure — the same
as nothing stopping `ChannelStreamTransport` being passed to `New` in a binary.

### Seams still deliberately **not** taken

| Would-be seam | Why not |
|---|---|
| `EnvelopeCodec` port | The protojson options (`UseProtoNames`, `EmitUnpopulated` / `DiscardUnknown`) and the `{"payload": …}` entry shape **are** the contract (ADR-0006/0008). One encoding on both sides is the point; a pluggable codec reintroduces the `system_config_update` protojson-vs-`encoding/json` asymmetry the rebuild killed. Hard-coded. |
| `PolicyProvider` interface | Already a `func() BusPolicy`. Its "adapters" (server closure over `appconfig.Get().EventBus`; agent `DefaultBusPolicy`; test fixed value) are all a func returning a struct. `BusPolicyFromConfig(*metarrv1.EventBusConfig)` stays a wiring-layer mapping so the generated proto type never enters the `Bus` core. |
| `Now` as an interface/port | Real seam (2 adapters: system clock, test clock) but a one-method one — a `func() time.Time` field, not a named interface. |

Net: **two real ports (`StreamTransport`, `PubSubTransport`)** — each with a
production Redis adapter and an in-memory test adapter, so the whole `Bus` is
exercisable with no Redis — plus one function-typed seam (`Now`). Everything else
is an internal detail or fixed, on purpose. That restraint is the maintainability
property this design is chosen for.

---

## 5. Behind the seam (adapter-independent)

Logic the `Bus` owns identically whether `StreamTransport` is Redis or gochannel:

- **Envelope assembly from the row** — `Source = cfg.Source`, `Name` validated
  against `topic.Events`, `Timestamp = cfg.Now()`, `correlation_id` threaded,
  payload attached; then protojson.
- **Per-`(topic, name)` dispatch** — one Watermill `AddNoPublisherHandler` per
  stream; its handler decodes the envelope once and indexes the registered map by
  `event.Name`.
- **The unknown-name default, in one place** — index miss → `logger.Warn` with
  stream + name → ack (not an error, so not retried). No listener re-implements it.
- **Reply-channel derivation + correlation stamping** — `ReplyChannel(id) =
  "reply." + id`; `Request` subscribes there and waits for the SUBSCRIBE ack before
  publishing; `HandleRequest` assembles the reply (`cfg.Source`, request
  `correlation_id`, `topic.ReplyName`) and publishes there.
- **Failure convention** — the `Recoverer → dropAfterRetry → Retry` middleware
  stack, built from `Policy().Retry`, on the `Bus` side of the port, so the rules
  have one implementation regardless of adapter.
- **Retention orchestration** — every `Publish` sets `MAXLEN ~
  Policy().Retention.MaxLen`; when `cfg.RetentionSweep`, `Run` spawns the goroutine
  that every `Policy().SweepInterval` runs `DiscoverTopics` (streams only) + `XTRIM
  MINID` at `cfg.Now() - RetentionHours`. Callers never construct a `RetentionSweeper`.
- **Policy late-binding** — `cfg.Policy` read in `Run` and per sweep, never at
  `New`; the `redisstream.Publisher` (its `DefaultMaxlen`) and the `Retry`
  middleware are built only in `Run`. Collapses the server's build-twice startup.
- **Consumer-identity derivation** — `SourceServer → ConsumerName`,
  `AgentSource(slug) → slug`.
- **Publishable-topic guard** — pattern rejected, off-table name rejected
  (`streamTopicPublishable`, folded into `Publish`).
- **No-op client close** — `Close` stops the router, the Pub/Sub subs, and the
  `StreamPublisher`; it never touches `cfg.Redis`.

---

## 6. Dependency & test strategy

| Dependency | DEEPENING category | Strategy | Test double |
|---|---|---|---|
| Redis client (Pub/Sub half) | 3 | **port `PubSubTransport`** (added 2026-09-02; prod default `RedisPubSub`, near side of the port keeps the ack + reply-routing logic) | `InMemoryPubSub` (in-process broker) |
| Redis client (retention sweep XTRIM/SCAN, stream transport construction) | 3, one impl | `cfg.Redis` held directly; no port | miniredis (`bus_miniredis_test.go`) |
| Watermill publisher + subscriber | 3 | **port `StreamTransport`** | `ChannelStreamTransport` (gochannel) |
| Watermill `message.Router` + `Recoverer`/`dropAfterRetry`/`Retry` | 1 | owned by the `Bus`, near side of the port — rules have one implementation | — |
| `metarrv1.EventBusConfig` | 1 | `BusPolicyFromConfig` at the wiring layer; `Bus` core takes `BusPolicy` | — |
| policy value | 1 | late-bound `func() BusPolicy` | fixed-value func |
| clock | 1 | `cfg.Now func() time.Time` | fixed / advanceable clock |
| the wire contract | 4 (conform, don't mock) | protojson opts + `{"payload": …}` hard-coded | — |

`router_miniredis_test.go` / `streambus_test.go` stay on miniredis for the
consumer-group / `XAUTOCLAIM` behaviours `ChannelStreamTransport` does not model.
Per the "replace, don't layer" rule, unit tests on the four old types are deleted
once tests at the `Bus` interface cover the behaviour.

---

## 7. Call-site deltas

- **`main.go` (both binaries):** one `eventbus.New(Config{…})`, N `Register*` calls,
  one `go bus.Run(ctx)`. Deleted: the bootstrap `StreamBus` and its rebuild, the
  `busPolicy` local threaded through four constructors, `NewPubSubBus`,
  `NewPubSubRouter`, `NewRedisRouter`, the standalone `RetentionSweeper` goroutine
  (folded into `Run`), the `startRouter` closure with its two `.Run` calls.
- **`appconfigstore.New`:** takes the `Bus` as its update publisher; constructed
  once.
- **Every stream listener:** `bus.HandleStream(topic, map[string]StreamHandler{…})`;
  the `switch event.Name { … default: warn }` blocks in
  `agent_scan_result_listener.go`, `system_config_update_listener.go`, and
  `scanner.go` are deleted.
- **Every durable publish** (`services/tasks.go`, `appconfigstore/store.go`,
  `scanner.go`): `bus.Publish(ctx, topic, name, correlationID, payload)` — no
  `NewEvent`, no `SourceServer` / `AgentSource(slug)` literal.
- **Responders** (`heartbeat_listener.go`, `nfo_reader.go`): `bus.HandleRequest(topic,
  handler)` — reply name from the row.
- **`topics.go`:** `StreamTopic` → `Topic`; add `Kind`, `ReplyName`; add the
  `KindNotify` / `KindRequestReply` rows; widen `AgentScanResultTopic().Events`;
  rename `DiscoverStreamTopics` → `DiscoverTopics`.
- **ADR-0007 topology derivation:** reads the unified `Topics()` list.

---

## 8. Deferred (not this work)

- **Participant identity / presence / registry** — `Source` stays an open string so
  it can be added without a breaking change (ADR-0008).
- **Language-agnostic conformance harness** — keep the `Bus` contract assertions
  separable from Redis-specific tests so the harness is a lift, not a rewrite, when
  the first non-Go participant lands (ADR-0008 §Considered).
- **Full listener reshape to the `config_propagator` decoded-payload shape** — this
  design enables per-`(topic, name)` dispatch; migrating every listener to that
  shape is report candidate 4.
- **`documentation/modules/design/pages/event_bus.adoc`** — the normative
  cross-language contract page, written from this design plus the wire details
  (exact reply-channel formula, entry bytes, protojson options, the `.vN` rule).
