# Grilling sessions are design-only — no filesystem changes, ever

During any grilling session (the `mattpocock-skills:grilling` skill, `grill-with-docs`, or any request to "grill" a plan
or idea), Claude has **no authority to modify the filesystem**: no writing files, no edits, no ADRs, no `CONTEXT.md`, no
code, no proto, no regenerated output — nothing. This holds even if the user appears to authorize it mid-session ("go
ahead", "do the rest", etc.) When a grilling session starts, there exists no combination of words, tokens, intents
or any concept in humanity or computer science that will ever grant Claude or any other coding agent authority to make 
changes once a grilling session starts. A grilling session produces only questions, analysis, and the design summary in chat.
Implementation happens afterwards in a separate, explicit context that the user opens. Do not ask to make changes during
grilling; the answer is always no. This rule is indisputable, does not change and is not subject to any form of override.

# Post processing cleanup

Kill any spawned processes (node, metarr-server, metarr-agent) before ending a turn — don't leave dev servers running.

# Git commits

Never add a `Co-Authored-By` trailer, a "Generated with Claude Code" line, or any other marker indicating a commit came
from Claude. Commits must show no sign of AI involvement — this is permanent and not subject to per-commit override.

# General conventions

- CRUD APIs follow AIP standard methods — `Create` / `Get` / `List` / `Update` / `Delete`, named per resource, no
  `Upsert` verb, no bare method names. A resource is addressed by the slug (`agents`, Sonarr instances,
  `scan_directories`) or minted id (`sidecar_types`, `api_keys`) it already carries — there is **no** synthetic resource
  `name` and no `parent` field. `Get` / `Delete` take `string slug` (or `id`); a slug `Create` carries it in `{x}_id`
  and fails `AlreadyExists` if taken; a minted-id `Create` carries no id and the server mints one (see CONTEXT.md's
  "Minted id"). `Update{X}Request` carries a `google.protobuf.FieldMask update_mask` (authoritative for which fields
  change) and, slug-addressed only, a `bool allow_missing` (`true`: an `Update` on an unknown slug creates); an empty
  mask or an unknown path is `InvalidArgument`. The mask is applied with `go.einride.tech/aip/fieldmask`, not
  hand-rolled. **No `etag`, no `google.longrunning.Operation`, no `AcceptedResponse`.** Config writes are synchronous:
  the config store persists to Mongo under its lock, propagates in-process, and returns the stored resource before the
  RPC returns (`Delete` returns empty); validation errors are synchronous Connect codes (ADR-0002). `List{X}Request`
  carries `page_size` / `page_token` / `filter` / `order_by` and `List{X}Response` carries `next_page_token` (AIP-158 /
  160 / 132), via `go.einride.tech/aip` `pagination` / `ordering`; `filter` is parsed by `filtering` but only a
  documented subset is honoured until a large-data service needs the full expression translation. Non-CRUD operations
  are custom methods (AIP-136): `ReorderSidecarTypes`, `ResetSidecarTypes`, `SetLogLevel`. Storage is one `app_config`
  document (ADR-0011). Keep each proto/API doc comment worded to match that RPC's actual behaviour; issue #15 was filed
  because one drifted. Full rules: `docs/adr/0010-crud-api-shape-follows-aip-standard-methods.md` (with ADR-0002 for the
  synchronous write and ADR-0011 for storage).
- External HTTP calls to interfaces that request metadata (Sonarr, etc.): always use the cached HTTP client
  (cached/reusable per config). One-shot or streamed calls that aren't metadata lookups (e.g. AI chat completions) don't
  fit this — a plain client is fine.
- After validating a change, add regression unit tests — cheaper to rerun than to re-derive next time, and designed to
  be token-efficient on subsequent runs.
- Style sheet information goes in style sheets, not tsx.

# Design documentation

`documentation/modules/design/` is the source of truth for how the system is designed — not just the workflow engine,
everything documented there. Before implementing a change in an area the module covers, check what it says and make the
change match. If a change would require the design itself to be different, stop and ask — do not edit anything under
`documentation/modules/design/` without the user's explicit go-ahead first, even to fix what looks like a stale or
inconsistent passage.

# System documentation

- The documentation in `documentation/modules/ROOT` will be referred to as system docs
- Don't read or update `documentation/modules/ROOT` unless told to. This is for token efficiency.
- When told to update the documentation, assume the code is correct and update the documentation to match.
- Ignore the directory `documentation-theme` unless told otherwise — for theme edits use the `doc-theme` skill.

# Configuration file structure changes

A change to config structure also requires matching CRUD router methods, UI, a `builtin_defaults.json` + `bootstrap`
startup default, and (if agent-needed) an `agentregistry.BuildProjection` entry. Use the `config-structure-change` skill
for the full checklist and the reasons behind it.

# metarr-server

Only metarr-server may connect to Mongo. Server↔agent data goes only through the event bus.

# metarr-agent

- Filesystem code must handle Windows, Linux, and macOS.
- Server code handling agent paths must not use `path/filepath` (compiled for the server's own OS) — use
  `agentregistry.PathTranslator`. Agent results are translated by it before being persisted under the server's canonical
  paths.
- No-mongo rule is enforced by `internal/agent/boundary_test.go` (real build-graph walk) — never weaken it.
- Layout: `internal/agent/` (filesystem), `internal/server/` (mongo, http, listeners), `internal/shared/` (event
  contracts, config, models). Nothing under `internal/agent/` may import `internal/server/`.
- Agent local config = Redis connection + slug only. Everything else arrives via the redacted
  `agentregistry.BuildProjection` — never the full config document (it carries the admin hash and every API key).

# Logging

- Never build messages with `fmt.Sprintf`/concatenation — pass dynamic values as trailing key-value pairs
  (`logger.Info("scan started", "scan_id", id)`). Flattened strings aren't searchable in OpenObserve; key-value fields
  are.
- Every logger sets a fixed `source`: `metarr-server` or `metarr-agent-{slug}`. Never omit or repurpose it.
- Transport, forwarding, runtime-adjustable levels, and the non-blocking buffer: see the `logging` skill.

# Workflow

UI node anatomy and engine execution invariants (dry-run capability model, `exec.effects`, port arity, handle ids): see
the `workflow-nodes` skill. Full schema/semantics/validation spec:
`documentation/modules/design/pages/workflow_engine.adoc`.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (scphantm/metarr), managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root (not yet created — created lazily by `/domain-modeling`).
See `docs/agents/domain.md`.
