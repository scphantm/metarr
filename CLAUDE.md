# Post processing cleanup
Kill any spawned processes (node, metarr-server, metarr-agent) before ending a turn — don't leave dev servers running.

# General conventions
* Descriptive names; standard clean-code practices.
* CRUD APIs: one upserting POST, never separate POST/PUT.
* External HTTP calls to interfaces that request metadata (Sonarr, etc.): always use the cached HTTP client (cached/reusable per config). One-shot or streamed calls that aren't metadata lookups (e.g. AI chat completions) don't fit this — a plain client is fine.
* After validating a change, add regression unit tests — cheaper to rerun than to re-derive next time.
* style sheet information should be stored in style sheets, not tsx.

# Configuration file structure changes
Any change to config structure also requires:
* CRUD methods in the config API router
* UI to manage the new settings
* init in `/cmd/metarr-server/main.go`
* if agent-needed: add to `agentregistry.BuildProjection` (readable by every agent host — add deliberately)

# metarr-server
Only metarr-server may connect to Mongo. Server↔agent data goes only through the event bus.

# metarr-agent
* Filesystem code must handle Windows, Linux, and macOS.
* Server code handling agent paths must not use `path/filepath` (compiled for the server's own OS) — use `agentregistry.PathTranslator`. Agent results are translated by it before being persisted under the server's canonical paths.
* No-mongo rule is enforced by `internal/agent/boundary_test.go` (real build-graph walk) — never weaken it.
* Layout: `internal/agent/` (filesystem), `internal/server/` (mongo, http, listeners), `internal/shared/` (event contracts, config, models). Nothing under `internal/agent/` may import `internal/server/`.
* Agent local config = Redis connection + slug only. Everything else arrives via the redacted `agentregistry.BuildProjection` — never via `system_config_update` (carries admin hash + every API key).

# Logging
* Never build messages with `fmt.Sprintf`/concatenation — pass dynamic values as trailing key-value pairs (`logger.Info("scan started", "scan_id", id)`). Flattened strings aren't searchable in OpenObserve; key-value fields are.
* Every logger sets a fixed `source`: `metarr-server` or `metarr-agent-{slug}`. Never omit or repurpose it.
* Log level is runtime-adjustable (System > Logging screen), never a restart-requiring constant — server: `appconfig.Logging.ServerLevel`; agent: its own `AgentConfig.LogLevel`.
* Logging never blocks: a call enqueues to a bounded buffer and returns immediately; a full buffer drops (and counts) the record.
* Neither binary talks to OpenObserve directly — both publish to the `logs.app` Redis Pub/Sub channel. Only `metarr-server` also subscribes and forwards over HTTP to Fluent Bit's `http` input (`internal/server/logforward`), via `logging.forward_url` in `config.yaml` (infra wiring, not `appconfig`). Fluent Bit has no Redis input plugin (verified) — hence the HTTP hop. Swapping vendors only touches `fluent-bit/fluent-bit.conf`'s OUTPUT block — no Go changes, no redeploys.

# Workflow UI node design
See `design.md` for the full spec (schema, execution semantics, validation). This is UI node anatomy only.
* Top edge: control-in (leftmost) then data-ins. Bottom edge: control-outs (leftmost) then data-outs. Right edge: `error`.
* Control edges (thick, solid, neutral, animated) show what runs next; data edges (thin, coloured by type) wire a value. Never style them the same.
* Input nodes have no control-in (starting points); output nodes have no control-out (ending points) — driven by the catalog's `control` block, never a hardcoded category check.
* `category` is presentation-only — never drives behavior. Dispatch on `type`.
* Arity: control out-port exactly one edge, data-in exactly one, data-out many, control-in many. Parallelism uses an explicit `core/parallel` node, never a second wire off one output.
* Handle ids encode port kind: `c:in`, `c:next`, `c:error`, `d:source`.
* Port `name` is a permanent id stored edges reference — renaming breaks saved workflows. Display text is `label`, free to change.

# Workflow engine
* Defaults to dry-run. Files can only be touched when dry-run is explicitly disabled (manual run, or automation in production mode).
* Dry-run is enforced by capability, not convention: a node handler never imports `os`/`os/exec` — it gets filesystem/process capability from the executor harness (`workflow.NodeContext`), and under dry-run those ops log-and-no-op rather than touch disk. A handler that forgets a flag check still can't write — it has no path to the filesystem. Never give a handler direct filesystem access "just this once."
* Every catalog entry must declare `exec.effects` (`read`|`write`|`destructive`) — missing it is a load error. Dry-run keys off this field; it can't be retrofitted without re-auditing every handler.
* The agent enforces dry-run itself too (the handler runs there) rather than trusting the server's decision.
