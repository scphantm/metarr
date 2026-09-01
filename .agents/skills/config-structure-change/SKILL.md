---
name: config-structure-change
description: The full checklist for any change to Metarr's config structure (the appconfig struct / config.yaml shape) — CRUD router methods, UI, the builtin_defaults.json + bootstrap startup default, and the agent projection. Use whenever adding, renaming, or removing a config field or section, instead of editing the struct and stopping there.
---

# Config structure changes

A change to config structure is not done until all of these land in the same change:

* **CRUD methods in the config API router** — one upserting POST (see the CRUD-API rule in `AGENTS.md`), never split POST/PUT.
* **UI to manage the new settings** — a screen or field under System settings.
* **A startup default:**
  * Add a section to `internal/shared/appconfig/builtin_defaults.json`.
  * Add a step in `internal/server/bootstrap` (see `docs/adr/0004-bootstrap-module-and-embedded-defaults-file.md`). A step that seeds a static value goes in the single consolidated `Store.Bootstrap` call — `staticConfigSteps` in `bootstrap.go` — not a new standalone call (issue #15).
  * Never a hand-written literal in `/cmd/metarr-server/main.go`. It calls `bootstrap.Run` once and owns no seeding logic.
* **If the field is agent-needed:** add it to `agentregistry.BuildProjection` (readable by every agent host — add deliberately). Never route it through `system_config_update`, which carries the admin hash and every API key.

## Why the startup-default rule is strict

Keeping `main.go` seeding-free makes `bootstrap` the single place config defaults are defined. Issue #15 was filed when a standalone bootstrap call drifted out of sync with the consolidated one.
