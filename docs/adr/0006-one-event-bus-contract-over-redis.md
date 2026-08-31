---
status: accepted
---

# The event bus is one contract over Redis: routed streams for durable work, Pub/Sub for the rest

The server↔agent event system grew one channel at a time. Durable work runs over Redis Streams consumed through Watermill, but only the bare subscriber, so consumer-group creation and stuck-message recovery are the library's while retry and dead-lettering are nobody's. Notifications and synchronous request/reply run over Redis Pub/Sub on a hand-rolled layer. Stream and channel names are declared twice, in `internal/shared/eventbus` and `internal/shared/agentproto`, kept equal by hand. `system_config_update` is published as protojson and consumed with plain `encoding/json`. No stream is trimmed. There is no retry cap, so a handler that returns an error is redelivered forever, and several handlers return `nil` on failures they cannot retry purely to stop the loop. None of it is tested.

The decision is to rebuild it as one contract before the workflow engine starts dispatching over it.

- **Redis stays.** It already backs sessions, cache, presence, stats, and log fan-out, and it is deliberately the store so that systems outside Metarr can subscribe to the event streams with nothing but a Redis client.
- **Durable work runs over Redis Streams**, consumed through a single Watermill `message.Router` per process with `Recoverer`, `Retry`, and `PoisonQueue` middleware. Messages that exhaust the retry limit land on `events.dead_letter` and are acked. The `return nil` workarounds go away.
- **Notifications and request/reply stay on Redis Pub/Sub** on the thin hand-rolled layer. Both transports are kept on purpose; planned features need each.
- **One file in `eventbus` owns every stream, channel, group, and key name.** `agentproto` keeps the payload type aliases and slug helpers and imports the names.
- **The envelope is a proto message** encoded with protojson (`UseProtoNames`, `EmitUnpopulated`), with fields `name`, `source`, `correlation_id`, `timestamp`, `payload`. Inner payloads are proto messages, decoded by `name`. A breaking payload change gets a `.vN` suffix on `name` so both shapes coexist during a cutover.
- **Retention and retry limits live in a new `event_bus` section of the application config.**

## Why

The forcing function is the workflow engine (`documentation/modules/design/pages/workflow_engine.adoc` §8), which dispatches node execution to agents over this bus and needs a retry policy, a dead-letter path, and idempotent result handling that the current system does not have. Building that on top of a bus whose failure handling is "redeliver forever, or return `nil` and hope you published the failure first" would spread the problem, not contain it.

Watermill is kept rather than replaced. Hand-rolling the streams layer means owning `XAUTOCLAIM` reclaim and ack-after-crash races, which is exactly the code where a subtle mistake produces silent loss or duplication. Adopting the `Router` and its middleware gets a tested dead-letter and retry path, collapses each listener from a decode-and-ack loop to a registration call, and makes handler-logic tests run against Watermill's in-memory `gochannel` with no Redis. The cost accepted is more Watermill in the codebase: the `Router` lifecycle, middleware ordering, and `message.Message` context helpers become load-bearing.

## The external-subscriber contract

Stream entries use Watermill's default marshaller. Each entry has a `payload` field holding the envelope as JSON; the `_watermill_*` fields are the library's and carry no contract. An outside consumer reads `payload`, decodes the JSON, and switches on `name`. It can create its own consumer group on any stream, or tail with `XREAD`, without affecting Metarr's own consumers.

A custom marshaller that hoisted envelope fields to first-class stream fields was considered and rejected. Redis Streams have no server-side field filtering, so it would buy nicer field names and nothing functional, in exchange for a format seam Metarr would own on behalf of every external integration.

Every event stream retains at least 48 hours of history, or its `MAXLEN` in entries, whichever is larger. Two mechanisms enforce this: every publish sets an approximate `MAXLEN ~` chosen by stream tier — a higher cap for the agent result streams (`events.agent_scan_results`, `events.agent_node_results`), a default cap for the rest — and a periodic sweep issues `XTRIM MINID` against `now - 48h` on every known stream, so a low-volume stream is bounded by age as well as by count. A consumer offline longer than the window may miss events and reconciles through the query APIs. The caps and the window are hardcoded constants for now (`eventbus.DefaultRetentionPolicy`) and move into the `event_bus` config section.

## Delivery and failure

Streams are at-least-once. A handler returns an error only when it could not process the message at all, for example the payload would not decode or Mongo was unreachable. Work that ran and produced a failure is a result, not an error: the handler publishes a failure result event and returns `nil`, and never touches the retry path. This is how the workflow engine's "never Nack" rule (§8.4) is honoured without a mechanism behind it.

An error is retried a bounded number of times with exponential backoff, then republished to `events.dead_letter` with the reason `PoisonQueue` records, and acked so it stops cycling. `events.dead_letter` has no consumer. It is read with `redis-cli` when something needs investigating, and an entry is replayed by re-adding it to its origin stream after the cause is fixed. Automated replay tooling is out of scope until there is operational need for it.

## Consequences

- **Still one server process.** Every stream consumer on the server uses the consumer name `metarr-server`, defined once. A second instance would double-consume the config stream and break the single-writer assumption ADR 0002 records. Lifting that needs partitioned consumers or a distributed lock, not covered here.
- **The bus has two halves with different machinery.** Durable streams go through the `Router`; notifications and request/reply are hand-rolled on Pub/Sub. This is deliberate. The incoherence being fixed was a half-wired library, not the existence of two transports.
- **`events.sonarr_cache_data` is removed.** It fired an empty event that a listener logged and recorded, with no agent and no processing. It returns as a real stream when the feature it stood in for exists.
- **`workflow_engine.adoc` §14.3 is stale.** It says node status streams over the `wsbus` + `useTopic` WebSocket layer, which has since been retired for gRPC-Web server streaming. The engine's live progress goes over a per-run Pub/Sub channel as §8.3 already specifies. The design doc is left as-is pending its own revision.

## Out of scope

Replacing Redis. Running more than one server instance. The workflow engine's Redis working memory (`metarr:run:{runID}:out:{nodeID}:{frame}`), which is scratch state, not events. OpenTelemetry tracing across the bus. Dead-letter replay tooling.
