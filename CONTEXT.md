# Metarr

Metarr manages metadata for a media library. A server owns the database and the
configuration; agents run on the machines that hold the files, scanning them and
reading or writing sidecar metadata. Work is described as workflows and executed
by nodes.

## Language

### Configuration

**Application config**:
The single document holding every runtime-adjustable setting for the system. One
document exists; there is no second copy and no per-environment variant.
_Avoid_: settings, app settings, system config

**Config store**:
The one module through which every change to the application config passes. It
reads the current document, applies the change, and announces it. Its own read
capability exists only to serve startup bootstrap, before live config exists —
general server code never calls it.
_Avoid_: config service, config writer, config repo

**Live config**:
The in-process copy of the application config every server-side read (outside
bootstrap) uses. Kept current by the config store's mutation listener, which
writes it only after a mutation is durably persisted — never read directly
from storage.
_Avoid_: cached config, current config, config snapshot

**Config mutation**:
A single named change to the application config, applied to a document the config
store has just read. A mutation names what it changes; it never supplies a whole
document.
_Avoid_: config update, config write

**Bootstrap**:
The one-time seeding of the application config at server startup, before the
config store's mutation path is live to fire or consume events. Applied
synchronously, straight to storage — it is not a mutation and never fires
`system_config_update`, because nothing is running yet to consume it.
_Avoid_: config mutation, seed data, startup config

**Projection**:
The redacted per-agent view of the application config. An agent reads its own
projection and never the document itself, which carries every credential.
_Avoid_: agent config, agent view

### Event bus

**Event bus**:
The Redis-backed path through which the server, agents, and participants
exchange everything. No side calls another directly. It has two halves: durable
streams for work that must survive a restart, and Pub/Sub for notifications and
one synchronous pattern.
_Avoid_: message queue, broker, Redis

**Envelope**:
The fixed outer shape every event shares: `name`, `source`, `correlation_id`,
`timestamp`, and an inner `payload`. A consumer dispatches on `name`; a breaking
change to a payload gives its `name` a `.vN` suffix so both shapes can coexist.
_Avoid_: message header, event metadata

**Command stream**:
The durable per-agent stream carrying work the server has assigned to one agent.
It survives an agent restart, and the agent consumes it on return.
_Avoid_: task queue, job queue

**Result stream**:
A durable stream carrying outcomes from agents back to the server, one per kind
of work. Scans and workflow nodes have separate result streams so one kind's
backlog cannot hide another's.
_Avoid_: response queue, callback stream

**Stream topic**:
One row describing a durable stream: its name, the consumer group that reads
it (empty when nothing consumes it), whether a listener is registered
(_consumed_), and the event names a handler on it may see. One list of stream
topics is the single representation of every durable stream — the stats
dashboard, the retention sweep, the publish cap and per-agent discovery each
read it. A _pattern topic_ carries a glob instead of a literal name; the
per-agent command streams are its concrete rows, discovered against live
Redis with each group derived from the slug.
_Avoid_: catalog, registry, known streams, Kafka topic

**Request/reply**:
The one synchronous pattern on the bus. The server publishes a request on an
agent's Pub/Sub channel and waits on a correlation-scoped reply channel with a
timeout. Used only where a miss should fail fast rather than be retried. The
answering side is a responder.
_Avoid_: RPC, sync call

**Responder**:
The handler registered on a request channel that produces the reply a
request/reply call is blocked on. One per request channel on the answering
process — server-side for the heartbeat check, agent-side for an NFO read. It
receives the decoded request envelope and returns a reply payload; the bus
stamps the correlation id so the reply lands on the caller's correlation-scoped
channel.
_Avoid_: listener, handler, callback

**Bus snapshot**:
The single in-process record of the bus's live state: per durable stream its
depth, consumer groups, lag, pending count, oldest-pending age and
publish/consume rate; per Pub/Sub channel its subscriber count; plus Redis
server counters. Each numeric metric carries a few minutes of rolling history.
One snapshot exists per server process and every dashboard viewer reads it, so
Redis load does not grow with the number of viewers.
_Avoid_: metrics, stats dump, dashboard state

**Sampler**:
The one background loop on the server that polls Redis on a fixed interval and
writes the bus snapshot. It is the only thing that reads Redis for dashboard
data — the streaming dashboard fans the shared snapshot out to viewers rather
than each connection polling.
_Avoid_: poller, collector, scraper, metrics agent

**Expected subscriber**:
An identity — the server, or `agent-<slug>` — that the derived topology says
should be attached to a channel or stream, computed from the list of stream
topics and the agent registry, not observed from Redis. A row whose live count
is below its expected subscribers is flagged and kept on screen instead of
vanishing; presence keys say which expected agent is missing.
_Avoid_: listener count, connected client, actual subscriber

**Purge**:
The operator action that clears a jammed durable stream from the dashboard: an
approximate trim of every current entry, then a fast-forward of each consumer
group to `$`. The stream key and its groups stay in place, so consumers resume
with no redelivery and nothing left pending. Streams only — Pub/Sub buffers
nothing, so a channel cannot be purged.
_Avoid_: flush, drain, delete stream, clear queue

**Bus contract**:
The language-agnostic definition of everything needed to use the event bus: the
envelope message and every inner payload, the stream / channel / group / key
names, and the conventions proto cannot express — reply-channel derivation, how
`correlation_id` flows, the retention and at-least-once guarantees, the `.vN`
payload-evolution rule, and the one-field stream-entry shape. Versioned as the
`metarr.bus.v1` proto module plus its written conventions.
_Avoid_: wire format, bus API, event schema, protocol

**Participant**:
A process that implements the bus contract but is not part of Metarr — a
third-party integration or extension in any protobuf-capable language. It reads
streams, publishes events, and answers request/reply exactly as the server and
agents do. Its identity and presence model is not yet defined.
_Avoid_: client, external subscriber, plugin, consumer

**Reference adapter**:
The Go implementation of the bus contract, in `internal/shared/eventbus`, kept
as the worked example every other-language implementation is checked against.
Because it is the reference it carries no behaviour the contract does not
state: it stamps `source` and event `name` from the stream-topic row, never
from the call site.
_Avoid_: bus client, bus wrapper, facade

### Identity

Two idioms exist, and which one a thing uses is a decision, not an accident.

**Slug**:
A short human-chosen name that identifies a thing the operator named — an agent,
a scanner, a Sonarr instance. Unique, stable, and typed by a person, so it can
appear in paths and channel names.
_Avoid_: name, key, handle

**Minted id**:
An opaque identifier the server generates once and never reuses, for a thing with
no natural name — a sidecar type, an API key entry. It survives renaming, so an
entry stays addressable when every visible field changes.
_Avoid_: uuid, guid

**API key entry**:
One issued key, held in an access-level group. Its name is optional and not
unique, so it is identified by a minted id.
_Avoid_: token, credential

### Models

**Cross-language model**:
A data shape that has to be understood the same way on both sides of a language
boundary — the Go server and the TypeScript UI, or a running process and the
document it stores. The application config, scan records, the workflow catalog
are all cross-language models.
_Avoid_: DTO, wire type, shared type

**Generated model**:
A cross-language model whose one definition lives in proto, with the Go and
TypeScript forms produced from it by the build. The Go type is an alias to the
generated message, so the shape the service layer works with is the shape the
store persists — there is nothing between them to keep in step.
_Avoid_: proto type, message type

**Hand-written mirror**:
A second, by-hand copy of a cross-language model — a Go struct transcribed into
a TypeScript type, or into a proto message, kept aligned by discipline. The
defect a generated model replaces: nothing tells a mirror when the thing it
mirrors has changed. An architecture test now rejects new ones.
_Avoid_: shadow type, parallel type
