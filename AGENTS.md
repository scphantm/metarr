# Post processing cleanup
Kill any spawned processes (node, metarr-server, metarr-agent) before ending a turn — don't leave dev servers running.

# Git commits
Never add a `Co-Authored-By` trailer, a "Generated with Claude Code" line, or any other marker indicating a commit came from Claude. Commits must show no sign of AI involvement — this is permanent and not subject to per-commit override.

# General conventions
* CRUD APIs: one upserting POST, never separate POST/PUT. An empty id in the request means create (server mints the id); a non-empty id that doesn't match an existing entry is rejected as not found, never treated as a client-chosen id to create under — ids are always server-minted (see CONTEXT.md's "Minted id"). Keep the proto/API doc comment worded to match this exactly; issue #15 was filed because one drifted.
* External HTTP calls to interfaces that request metadata (Sonarr, etc.): always use the cached HTTP client (cached/reusable per config). One-shot or streamed calls that aren't metadata lookups (e.g. AI chat completions) don't fit this — a plain client is fine.
* After validating a change, add regression unit tests — cheaper to rerun than to re-derive next time, and designed to be token-efficient on subsequent runs.
* Style sheet information goes in style sheets, not tsx.

# Design documentation
`documentation/modules/design/` is the source of truth for how the system is designed — not just the workflow engine, everything documented there. Before implementing a change in an area the module covers, check what it says and make the change match. If a change would require the design itself to be different, stop and ask — do not edit anything under `documentation/modules/design/` without the user's explicit go-ahead first, even to fix what looks like a stale or inconsistent passage.

# System documentation
* The documentation in `documentation/modules/ROOT` will be referred to as system docs
* Don't read or update `documentation/modules/ROOT` unless told to.  This is for token efficiency.
* When told to update the documentation, assume the code is correct and update the documentation to match.
* Ignore the directory `documentation-theme` unless told otherwise — for theme edits use the `doc-theme` skill.

# Configuration file structure changes
A change to config structure also requires matching CRUD router methods, UI, a `builtin_defaults.json` + `bootstrap` startup default, and (if agent-needed) an `agentregistry.BuildProjection` entry. Use the `config-structure-change` skill for the full checklist and the reasons behind it.

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
* Transport, forwarding, runtime-adjustable levels, and the non-blocking buffer: see the `logging` skill.

# Workflow
UI node anatomy and engine execution invariants (dry-run capability model, `exec.effects`, port arity, handle ids): see the `workflow-nodes` skill. Full schema/semantics/validation spec: `documentation/modules/design/pages/workflow_engine.adoc`.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (scphantm/metarr), managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root (not yet created — created lazily by `/domain-modeling`). See `docs/agents/domain.md`.
