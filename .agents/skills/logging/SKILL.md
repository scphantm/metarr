---
name: logging
description: How Metarr's logging pipeline is wired — the Redis logs.app Pub/Sub channel, the server-only forward hop to Fluent Bit, runtime-adjustable levels, and the non-blocking bounded buffer. Use when touching internal/server/logforward, changing the log transport, swapping the log vendor, or debugging why logs don't reach OpenObserve. The two everyday coding rules (key-value pairs, fixed source) stay in AGENTS.md.
---

# Logging pipeline

The two everyday rules — key-value pairs instead of `fmt.Sprintf`, and a fixed `source` per logger — are in `AGENTS.md`. This skill is the transport and runtime detail behind them.

## Transport

* Neither binary talks to OpenObserve directly. Both publish to the `logs.app` Redis Pub/Sub channel.
* Only `metarr-server` also subscribes to `logs.app` and forwards over HTTP to Fluent Bit's `http` input (`internal/server/logforward`), via `logging.forward_url` in `config.yaml` — infra wiring, not `appconfig`.
* Fluent Bit has no Redis input plugin (verified) — hence the HTTP hop.
* Swapping vendors only touches `fluent-bit/fluent-bit.conf`'s OUTPUT block — no Go changes, no redeploys.

## Runtime behavior

* Log level is runtime-adjustable (System > Logging screen), never a restart-requiring constant — server: `appconfig.Logging.ServerLevel`; agent: its own `AgentConfig.LogLevel`.
* Logging never blocks: a call enqueues to a bounded buffer and returns immediately; a full buffer drops (and counts) the record.
