---
name: config-structure-change
description: The full checklist for any change to Metarr's config structure (the appconfig struct / config/server.yaml shape) — CRUD router methods, UI, the builtin_defaults.json + bootstrap startup default, and the agent projection. Use whenever adding, renaming, or removing a config field or section, instead of editing the struct and stopping there.
---

# Config structure changes

A change to config structure is not done until all of these land in the same change:

* **CRUD methods in the config API service** — AIP standard methods, named per resource, no `Upsert`: a new collection section needs `Create` + `Update` + `List` (paginated / filterable / orderable) + `Get` + `Delete`, addressed by the section's slug or minted id (no synthetic resource `name`, no `parent`); a new scalar section needs `Get` + `Update` with a `google.protobuf.FieldMask`. **No `etag`, no `google.longrunning.Operation`, no `AcceptedResponse`** — writes are synchronous, persist under the config store lock, and return the stored resource before the RPC returns. Field masks / pagination / ordering are `go.einride.tech/aip`. See the CRUD-API rule in `AGENTS.md` and `docs/adr/0010-crud-api-shape-follows-aip-standard-methods.md` (with ADR-0002 for the synchronous write, ADR-0011 for storage).
* **UI to manage the new settings** — a screen or field under System settings.
* **A startup default:**
  * Add a section to `internal/shared/appconfig/builtin_defaults.json`.
  * Add a step in `internal/server/bootstrap` (see `docs/adr/0004-bootstrap-module-and-embedded-defaults-file.md`). A step that seeds a static value goes in the single consolidated `Store.Bootstrap` call — `staticConfigSteps` in `bootstrap.go` — not a new standalone call (issue #15).
  * Never a hand-written literal in `/cmd/metarr-server/main.go`. It calls `bootstrap.Run` once and owns no seeding logic.
* **If the field is agent-needed:** add it to `agentregistry.BuildProjection` (readable by every agent host — add deliberately). That redacted projection is the only config an agent sees; never widen it to carry a credential (the admin hash, an API key, a Sonarr key).

## Why the startup-default rule is strict

Keeping `main.go` seeding-free makes `bootstrap` the single place config defaults are defined. Issue #15 was filed when a standalone bootstrap call drifted out of sync with the consolidated one.
