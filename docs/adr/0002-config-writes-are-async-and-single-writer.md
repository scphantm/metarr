---
status: accepted
---

# Configuration writes stay asynchronous, serialised by a single-instance lock, guarded by an etag

The config store's `Mutate` operation queues a change by firing a
`system_config_update` event and returning; it does not wait for the change to
be persisted. Persistence still happens later, in the existing
`system_config_update` stream listener. `Mutate` serialises itself with an
in-process `sync.Mutex` held across its own read, apply, and fire — not across
the listener's later write. The async write is surfaced to the caller as a
`google.longrunning.Operation` (AIP-151), and a stale-read lost update is caught
by an `etag` compare (AIP-154).

## Why

Two administrators changing different settings around the same time could
previously compute their changes from the same stale document and revert one
another, because each of the fifteen call sites read, mutated, and fired
independently with no ordering between them. `Mutate`'s lock closes that gap by
forcing one call's read to happen only after the previous call's fire has
completed, rather than by making the write itself synchronous.

Going fully synchronous — writing to MongoDB inside the RPC before responding —
was the alternative and is not what this does. It was rejected to keep the
change behaviour-preserving and to keep the config store a locking fix, not a
write-path rewrite. The event-sourced write path (fire, then a listener
persists) is unchanged.

**The lock does not cover every lost update.** It orders one request's read
after the previous request's fire, but a client that read the document minutes
ago and submits an edit computed from that stale copy is not protected — the
lock was not held across the human's think time. AIP-154's `etag` closes this:
every config resource and scalar section carries an `etag` that is a hash of its
stored bytes, `Update` and `Delete` echo back the etag the client last read, and
the mutation closure recomputes the current hash under the lock and returns
`ABORTED` on a mismatch. The etag is derived, never stored (ADR-0005), so it
adds nothing to the document; an empty etag on a request is a deliberate blind
write and skips the check.

**"Queued" is a `google.longrunning.Operation`, not a bare acknowledgement.**
The RPC records an operation keyed by the correlation id it already stamps on
the fired event — `operations/{correlation_id}` — and returns it with
`done:false`. The `system_config_update` listener, once it has persisted, marks
that operation `done`: `response` set to the resource on success, `error` set to
a `google.rpc.Status` when persistence or a late cross-entry validation fails.
The caller polls `OperationsService.GetOperation`. This is what lets a
persistence failure — an invalid sidecar type table caught only when the
listener rebuilds the registry — reach the original caller, which the old
"queued, poll the resource yourself" acknowledgement structurally could not.

## Consequences

- **The lock is correct only while one server process runs.** Its serialisation
  is process-local; a second server instance would not share it, and two
  instances could still race the way single-process concurrent requests used to.
  This matches what the codebase already assumes — every stream listener uses a
  hardcoded consumer name (`"worker-1"`), and no configuration for running more
  than one server instance exists. If that ever changes, the lock needs a
  distributed lock or a document revision in its place; the `etag` compare keeps
  working unchanged, since it is content-based rather than lock-based, and would
  become the primary conflict guard.
- **A caller is told "accepted," and can then learn the outcome.** The RPC
  returns an operation, not the stored resource. A failure during the listener's
  later persistence surfaces as `operation.error` on `GetOperation`; a success
  surfaces as `operation.response`. The UI's queued→confirmed indicator still
  re-reads the resource until it reflects the write — the same eventual
  consistency the operation reports — and moves to polling the operation once
  every config write returns one.
- **The lock does not order the eventual persistence, only the reads that
  precede it.** Two `Mutate` calls close enough together that the listener has
  not yet persisted the first one's event before the second one reads could
  still race on overlapping fields; the second one's `etag` catches it if the
  client is working from the pre-first-write read, and the operation for the
  losing write reports `ABORTED`. Non-overlapping edits are unaffected.
- **A minted-id `Create` learns its id from the operation.** The id is not known
  when the RPC returns; the client reads `operation.response` once `done`,
  rather than re-listing the collection.
