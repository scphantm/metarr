# Post processing cleanup
Kill any spawned processes (node, metarr-server, metarr-agent) before ending a turn — don't leave dev servers running.

# Git commits
Never add a `Co-Authored-By` trailer, a "Generated with Claude Code" line, or any other marker indicating a commit came from Claude. Commits must show no sign of AI involvement — this is permanent and not subject to per-commit override.

# General conventions
* CRUD APIs follow AIP standard methods — `Create` / `Get` / `List` / `Update` / `Delete`, no `Upsert` verb. `Update{X}Request` carries a `google.protobuf.FieldMask update_mask` (authoritative for which fields change) and a `bool allow_missing`: `true` for slug-addressed sections (an `Update` on an unknown slug creates), left `false` for minted-id sections (an `Update` on an unknown id is `NotFound`). A slug-addressed `Create` takes the operator's slug in `{x}_id` and fails `AlreadyExists` if it is taken; a minted-id `Create` carries no id and the server mints one (see CONTEXT.md's "Minted id"). Every resource and scalar section carries an `OUTPUT_ONLY` `etag` (a hash of its stored bytes); `Update` / `Delete` echo the last-read etag back and a mismatch is `ABORTED` (AIP-154). `List{X}Request` carries `page_size` / `page_token` / `filter` / `order_by` and `List{X}Response` carries `next_page_token` (AIP-158 / 160 / 132). Config writes return a `google.longrunning.Operation` (name `operations/{correlation_id}`), never the resource — they are eventually consistent (ADR-0002); poll `OperationsService.GetOperation` for `done` + `response` / `error`. The AIP plumbing — resource names, field masks, pagination, filtering, ordering — is `go.einride.tech/aip`, not hand-rolled. Keep each proto/API doc comment worded to match that RPC's actual behaviour; issue #15 was filed because one drifted. Full rules: `docs/adr/0010-crud-api-shape-follows-aip-standard-methods.md`.
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
