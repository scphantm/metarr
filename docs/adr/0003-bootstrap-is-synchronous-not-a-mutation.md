---
status: accepted
---

# Startup bootstrap is a separate synchronous store capability, not a config mutation

Server startup seeds the application config (API keys, admin credentials, admin lockout recovery, directory-scanner defaults, sidecar types, agent normalization, logging defaults, API-key id backfill) before anything else runs. This seeding is applied through new synchronous methods on `appconfigstore.Store`, sharing its single-writer mutex, but distinct from `Mutate`: it persists directly and fires no `system_config_update` event. `CONTEXT.md` names this **Bootstrap**, explicitly not a **config mutation**.

## Why

`Mutate` (docs/adr/0002-config-writes-are-async-and-single-writer.md) is deliberately asynchronous: it fires an event and returns, and actual persistence happens later in the `system_config_update` stream listener. That listener does not start until after the point in `cmd/metarr-server/main.go` where bootstrap runs. Several services (`directory_scanner.go`, `sonarr_interfaces.go`, `agents.go`, `logging.go`, `tasks.go`, `config.go`) read the application config straight from Mongo per call, bypassing the in-memory `appconfig.Get()` global that the listener keeps current. Routing bootstrap through `Mutate` as-is would let the server start accepting requests while Mongo still held the pre-bootstrap document — a real race window, not a theoretical one, for every one of those services.

ADR-0002 rejected making `Mutate` synchronous to keep that change a narrow locking fix. Bootstrap has the opposite requirement from the runtime path: nothing else is running yet to race it, but request handlers need Mongo already caught up before they start serving. A synchronous, non-event contract fits that requirement without touching `Mutate`'s.

## Considered options

- **Reuse `Mutate` as-is for bootstrap**: rejected — the listener isn't running yet, so the write wouldn't land in Mongo before the server starts serving.
- **Start the listener before bootstrap and add a server-side wait-for-confirmation poll after each `Mutate` call**: rejected — keeps one write contract everywhere, but requires new polling plumbing the server side doesn't have (only the UI polls today, via `useConfirmationPoll`), and makes startup block on stream round-trips for no benefit over a direct synchronous write.

## Consequences

- `Store` now has two contracts sharing one mutex: the async runtime path (`Mutate`, `Read`) and the sync startup path (`Bootstrap`-style calls, plus an order-enforcing `SeedAdmin` for the one pair with real incident history — the admin-seed-before-lockout-recovery sequence from ADR-0001). Both must stay on the same type; splitting them would either duplicate the lock or force it to be exported.
- Bootstrap calls report only what they changed; nothing returns a whole document. `main.go` calls `Store.Read` once after all bootstrap calls complete to get the final document for `appconfig.Set` and `agentRegistry.PublishAll`.
- Each bootstrap call skips its own Mongo write when its closure reports no change, preserving the existing zero-writes-on-an-ordinary-restart behavior.
