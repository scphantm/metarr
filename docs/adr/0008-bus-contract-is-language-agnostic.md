---
status: accepted
---

# The event bus contract is language-agnostic, and Go holds the reference implementation

ADR-0006 rebuilt the server↔agent event system as one contract over Redis and noted, almost in passing, that Redis is "deliberately the store so that systems outside Metarr can subscribe to the event streams with nothing but a Redis client." That sentence is now a requirement. Metarr will grow an extension model in which a third-party process — written in any language with protobuf support — connects to the bus and is driven by the workflow engine to add behaviour the built-in nodes do not have. For that, the thing a caller implements has to be the bus contract itself, not a Go package. This ADR records that the contract is the module and Go holds only a reference implementation of it.

## Decision

- **The bus contract is a language-agnostic artifact.** It is the union of: the `EventEnvelope` message and every inner payload message; the stream, channel, consumer-group and key names; and the transport conventions proto cannot express — how a request/reply call derives its reply channel, how `correlation_id` flows, the retention and at-least-once guarantees, the `.vN` payload-evolution rule, and the stream-entry shape. A process that implements all of it is a **participant** and does everything the server and agents do on the bus: read streams, publish events, and answer request/reply.

- **The contract's proto lives in its own module, `metarr.bus.v1`.** The envelope, the bus payloads, and the existing server↔agent contract messages move out of the flat `metarr.v1` package into `metarr.bus.v1`, which carries the strict `buf breaking` guarantee and is the published, versioned surface. The internal CRUD, config, auth and stats service protos stay in `metarr.v1` and keep changing release to release without that being an external break.

- **Go gets one reference adapter.** `internal/shared/eventbus` presents a single `Bus` — one type, one `Run`, one lifecycle — replacing the four things a caller assembles today (`StreamBus`, `PubSubBus`, `Router`, `PubSubRouter`) plus the hand-built envelope and the topic constructors. Because it is the reference, it carries no behaviour the written contract does not state: it stamps `source` and the event `name` from the stream-topic row rather than from the call site, and it derives reply channels by the documented formula, but it invents nothing.

- **`source` stays an open string.** The contract does not enumerate the valid values of the envelope's `source` field. `metarr-server` and `agent-<slug>` are the ones in use; a participant identity and presence model will add more without a breaking change.

- **The Go rebuild folds in the two-phase startup.** The server currently builds the stream bus and the config store twice — once on `DefaultBusPolicy`, then again once the `event_bus` config section exists — because the retention policy is not known at construction. `Bus` takes the policy as a late-bound provider and is built once.

- **`StreamTopic.Events` becomes load-bearing.** The stream-topic row already lists the event names a handler on that stream may see; today the field is informational and every listener re-implements a `switch name { default: warn }`. The `Bus` router dispatches per `(topic, event name)` and owns the unknown-name path once. Existing listeners keep working through a shim; reshaping each listener to a decoded-payload handler is a follow-up.

## Scope

This ADR covers the contract and the Go reference adapter only. It does **not** design:

- the workflow-function extension mechanism — how the engine registers an external function, invokes it, and bounds its failures — which is a separate design on top of a finished contract;
- participant identity, presence, registry, dashboard topology, or contract-version negotiation, which ADR-0007's expected-versus-actual view will eventually need but which is deferred;
- a polyglot **agent** — the filesystem-scanning agent's full contract (command stream, result publishing, NFO responder, projection read, presence, path translation) is a much larger surface than the bus and is not opened here.

## Considered and rejected

- **Keep the depth in a Go `Bus` and let other languages mirror the rules by hand.** This is the hand-written-mirror defect ADR-0005 exists to remove, one language boundary further out: nothing tells a Python reimplementation that the reply-channel formula changed. The contract has to be the artifact both sides read.
- **Publish the flat `metarr.v1` module as-is.** An external author would then depend on `AdminUser`, every config service, and the stats messages, and `buf breaking` would gate the entire proto tree against the outside world. The module split is what lets internal messages churn freely while the contract stays still.
- **Binary protobuf on the wire.** Rejected in favour of the single protojson encoding ADR-0006 chose: every protobuf library can produce protojson, and a JSON `payload` field stays readable with `redis-cli` during an incident.
- **A language-agnostic conformance suite now.** There is no second implementation yet, so a real-Redis harness that exercises an arbitrary adapter is speculative. The Go tests keep the contract-conformance assertions separable so the harness is a lift, not a rewrite, when the first non-Go participant lands.

## Consequences

- The reply-channel derivation, the `correlation_id` rules, the `source` values in use, and the exact protojson options become a written spec Metarr owns on behalf of every integration; a casual change to any of them is a breaking change. `documentation/modules/design/pages/event_bus.adoc` is the designated reference; this ADR and ADR-0006 record the decisions.
- `internal/shared/agentproto` and the one file in `internal/shared/eventbus` that owns bus names alias and import from `metarr.bus.v1` generated code rather than `metarr.v1`.
- Watermill stays the stream consumer, but its default marshaller does not — see the ADR-0006 amendment of the same date: an external publisher cannot be asked to emit Watermill's `_watermill_*` framing.
- No credential may appear in a bus payload once a participant can attach with only the Redis credential — see ADR-0009.
- Still one server process. ADR-0006's single-writer consequence is unchanged; a participant is not a second server.
