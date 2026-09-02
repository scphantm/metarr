---
status: accepted
---

# Configuration writes are synchronous, serialised by a single-instance lock

The config store's `Mutate` reads the current document, applies one scoped
change, writes it to MongoDB, and propagates it to the rest of the process —
all while holding an in-process `sync.Mutex`. The RPC returns after the write
has landed. There is no event-bus hop in the write path and no separate
listener that persists later.

## Why

An earlier version of this decision kept the write asynchronous: `Mutate`
fired a `system_config_update` event and returned, and a stream listener in
the same process persisted it afterward. That was adopted because the change
was a locking fix layered onto an existing event-sourced path, and the intent
was to keep it a locking fix rather than a write-path rewrite. The config API
reshape (ADR-0010) is that rewrite, it is greenfield, and it removed the
reason to keep the async path. Routing the whole config document through a
Redis stream so the one server process could hand it back to itself was
overhead with no benefit: nothing reads config from Mongo at runtime — the
server reads an in-process singleton, agents read a Redis projection — so the
listener's only job was to update state the writing code can update directly.

**The lock stays.** Two administrators changing different settings around the
same time could otherwise compute their changes from the same document and
revert one another. `Mutate` holds its mutex across read, apply, write, and
propagate, so a concurrent `Mutate` cannot start its read until the previous
one has finished. This is process-local serialisation: it is correct only
while one server process runs, which is what the codebase already assumes
everywhere — every stream listener uses a hardcoded `worker-1` consumer name,
and no multi-instance configuration exists. If that ever changes, a document
`revision` compared under the lock, or a distributed lock, takes its place.

**No `etag`.** The lock serialises writes and `FieldMask` (ADR-0010) scopes
each one to the fields it names, so the only unguarded case is two writers
editing the same field of the same resource within the same window. With a
single admin account that is theoretical, and an `etag` on every message is a
token every client has to round-trip for it. A document `revision` is the
cheap retrofit if multi-writer operation ever becomes real.

## Propagation

After the Mongo write, still under the lock, `Mutate` propagates the new
config to the parts of the process that hold derived state: it swaps the
in-process live-config singleton, applies the server log level, recompiles the
sidecar classification registry, and republishes each agent's redacted
projection. Agent projections still travel over Redis — agents have no Mongo
access, and server↔agent data goes only through the bus. A failure in any
propagation step after a successful write is logged, not returned: the write
has landed and is authoritative, and the agents re-read on their own timer
regardless.

## Consequences

- **The `system_config_update` Redis stream, its consumer group, its event
  name, and `RegisterSystemConfigUpdateListener` are removed.**
  `config_propagator.Apply` becomes a plain function the store calls after the
  write, minus the persist step it used to own.
- **No long-running operations.** The write is done when the RPC returns, so
  there is no `Operation` to poll — `OperationsService`, the `Operation`
  message, and the `config_operations` Mongo collection are removed
  (ADR-0010).
- **A persistence failure reaches the caller synchronously**, as a Connect
  `Internal` on the call, instead of surfacing later on a polled operation.
  The in-process propagation runs only after a successful write, so a failed
  write leaves live config and every derived copy untouched.
- **`Bootstrap` and `Mutate` converge.** Both lock, read, apply, and write
  synchronously through the same repo. `Bootstrap` keeps its two specifics: it
  writes only when `apply` reports a change, and it does not propagate,
  because it runs before the rest of the process is up (ADR-0003).
- **The UI stops polling.** A write returns the stored resource and the client
  updates its cache from it; the queued→confirmed re-read loop is gone.
