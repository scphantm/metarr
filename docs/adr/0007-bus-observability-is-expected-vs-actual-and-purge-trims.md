---
status: accepted
---

# Bus observability is expected-versus-actual, and purge trims streams rather than deleting them

The `/system` dashboard is backed by a prototype (`internal/server/redisstats`) that reads Redis on every viewer connection and shows a flat set of numbers. It has no history, no notion of what *should* be attached versus what is, and no way to act on a jammed stream from the screen. When an agent drops a subscription its row vanishes instead of showing as broken, and a backed-up stream can only be cleared with `redis-cli`.

The replacement (scphantm/metarr#59) is a single background **sampler** that polls Redis on a fixed interval into one in-process **bus snapshot** with a short rolling history, which every dashboard viewer reads. This ADR records three decisions that shape that surface and that a later reader is likely to want to "fix" without the context for why they are the way they are.

## Per-identity data is expected-versus-actual, not per-publisher instrumentation

The dashboard answers "which identities should be attached to this channel or stream, and which are missing." It does that by deriving the expected topology as a pure function of the stream-topic list plus the set of registered agents, then comparing it against the live counts Redis reports (`PUBSUB NUMSUB`, `XINFO GROUPS`). A row whose live count is below its expected count is flagged and kept on screen; agent presence keys disambiguate which expected agent identity is the missing one.

The rejected alternative is to have every publisher and subscriber — server and every agent — report its own attachments and throughput over the bus, and assemble the dashboard from those reports. Redis does not map a subscription to the process holding it, so that is the only way to get true per-identity data. It was rejected because it turns observability into a second cross-binary protocol that every agent has to implement and keep correct, with its own failure modes, for a screen whose actual question — "is the listener I expect present" — is fully answered by topology-minus-liveness. The expected side is also then unit-testable with no Redis at all: given a stream-topic list and a set of slugs, the expected rows and their expected identities are a table.

The cost accepted is that "attached" on this dashboard always means *a live count at least as high as the expected count*, cross-checked against presence keys — never a verified identity. Two agents that both fail to subscribe to a shared channel show as one flagged row, and the presence keys are what say which two.

## Purge is streams-only, and works by approximate trim plus consumer-group fast-forward

An operator can purge a durable stream from the dashboard. Purge performs an approximate trim of every current entry (`XTRIM MINID` at roughly now) and then fast-forwards every consumer group on that stream past the trim (`XGROUP SETID` to `$`). It returns the number of entries dropped. It does **not** `DEL` the stream key and does **not** destroy the consumer groups.

`DEL` was rejected because it takes the consumer groups with it. Every consumer would then either recreate its group from `0` and re-read whatever arrives next from the beginning, or error until it recreates it, and a reserved stream would disappear from Redis entirely rather than read as empty. Destroying the groups individually has the same effect without removing the key. Trim-plus-`SETID` leaves the stream and its groups in place, fast-forwarded past the drop, so every consumer resumes cleanly with no redelivery and nothing left pending — which is the whole point of a purge during an incident.

The trim is approximate (`MINID ~`) because an exact trim scans the stream; the small overshoot is irrelevant when the intent is "drop everything up to now." Purge tolerates a stream that has no consumer groups yet: it trims and does nothing else, so a "purge all streams" batch does not break on a reserved stream (for example the workflow node-result stream, scphantm/metarr#37) that nothing consumes.

Purge is a streams-only concept. Redis Pub/Sub buffers nothing, so there is no channel backlog to clear and the dashboard deliberately gives channels no depth column and no purge control.

## Per-channel Pub/Sub throughput is dropped

The dashboard shows Pub/Sub channels with a subscriber count only — no publish rate, no consume rate. Redis exposes neither for a channel, and the only way to produce them is to have every publisher, agents included, count its own publishes per channel and report them. That is the same per-publisher instrumentation rejected above, for the same reasons, and it is out of scope here. Stream publish and consume rates *are* shown, because those come from deltas of counters Redis already keeps (`entries-added` on the stream, `entries-read` on the group) between two sampler passes, with no publisher cooperation.

## Relationship to ADR-0006

ADR-0006 rebuilt the bus as one contract and explicitly scoped bus observability tooling out: "OpenTelemetry tracing across the bus" is listed under *Out of scope*, and the rebuild adds no metrics stack. This ADR fills that gap with the smallest thing that holds up operationally — one in-process sampler, one shared snapshot, a dashboard derived from the same stream-topic table ADR-0006 established — and does not reverse ADR-0006's rejection of a metrics stack. There is still no Prometheus, no OpenTelemetry, and no persistent time series: the rolling history is in memory and lost on restart.

## Consequences

- The dashboard's per-identity view is only ever as accurate as the derived topology. A channel that gains a listener the stream-topic list and agent registry do not describe will not show that listener, by design.
- Purge leaves consumer groups fast-forwarded to `$`. A consumer that was mid-backlog loses that backlog with no dead-letter record — this is an incident tool, and the warn-level audit log entry (who, which stream, how many entries) is the only trace.
- The expected-topology derivation and the purge mechanics are both testable against miniredis or with no Redis at all, which is why they are pure functions over the stream-topic list rather than methods on a live sampler.

## Out of scope

Verified per-identity attachment (Redis cannot provide it). Per-publisher or per-channel throughput counters. A metrics stack (Prometheus, OpenTelemetry) — ADR-0006 scoped this out and this ADR does not reverse it. Persistent history for the rolling series. Any inspect or export view of stream contents; the only stream action is purge. Any agent-binary change — the agent does not report its own subscriptions.
