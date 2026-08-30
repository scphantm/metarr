---
status: accepted
---

# Configuration writes stay asynchronous, serialised by a single-instance lock

The config store's `Mutate` operation queues a change by firing an event and returning; it does not wait for the change to be persisted. Persistence still happens later, in the existing `system_config_update` stream listener, unchanged. `Mutate` serialises itself with an in-process `sync.Mutex` held across its own read, apply, and fire — not across the listener's later write.

## Why

Two administrators changing different settings around the same time could previously compute their changes from the same stale document and revert one another, because each of the fifteen call sites read, mutated, and fired independently with no ordering between them. `Mutate`'s lock closes that gap by forcing one call's read to happen only after the previous call's fire has completed, rather than by making the write itself synchronous.

Going fully synchronous — writing to MongoDB inside the RPC before responding — was the alternative and is not what this does. It was rejected here to keep this change behaviour-preserving: the existing poll-until-confirmed save flow the interface already relies on continues to work unmodified, and the scope of introducing the config store stays a locking fix, not a write-path rewrite.

## Consequences

- **The lock is correct only while one server process runs.** Its serialisation is process-local; a second server instance would not share it, and two instances could still race the way single-process concurrent requests used to. This matches what the codebase already assumes — every stream listener uses a hardcoded consumer name (`"worker-1"`), and no configuration for running more than one server instance exists. If that ever changes, the lock stops being sufficient and needs a document revision or a distributed lock in its place.
- **A caller is told "queued," not "stored."** A failure during the listener's later persistence — for instance an invalid sidecar type table — cannot reach the original caller. The interface's existing poll for confirmation is still how a client learns whether a change actually landed.
- **The lock does not order the eventual persistence, only the reads that precede it.** Two `Mutate` calls close enough together that the listener has not yet persisted the first one's event before the second one reads could still race. This is expected to be far rarer than the defect being fixed, since stream consumption is normally much faster than the gap between two independent requests, but it is not eliminated.
