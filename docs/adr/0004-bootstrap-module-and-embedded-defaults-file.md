---
status: accepted
---

# Startup seeding moves to a standalone `bootstrap` package backed by one embedded defaults file

Startup seeding (docs/adr/0003) grows from eight closures inline in `main.go`, plus hand-written Go literals for defaults (`appconfig.Default()`, `DefaultSidecarTypes()`), into a standalone `internal/server/bootstrap` package. Each seed is a `Step` — a predicate plus an apply function, run in a fixed order with no dependency graph. Tracing all eight existing steps found no real cross-step ordering dependency once `SeedAdmin`'s one genuine case (seed-vs-recover) is accounted for, and that case is already folded into a single atomic closure — a dependency-aware runner would be solving a problem that doesn't exist here. Static defaults (directory-scanner settings, sidecar types, agent/logging defaults, admin's username/email) move out of Go literals into one `go:embed`-compiled JSON file with a section per setting group. `appconfig.Default()` and `DefaultSidecarTypes()` are rewritten to read the same file, closing a drift already visible between them: `Default()` never set `DirectoryScanner.ParallelCount`, while the bootstrap block set it to 16. Fields needing a fresh value per install (API key IDs and secrets) use a `{guid}` placeholder in the JSON, substituted with an independently generated UUID per occurrence via a parsed-tree walk — never a raw string replace, which would stamp the same UUID into every occurrence.

## Why not grow `appconfigstore.Store` instead

ADR-0003 already established a precedent for this exact problem: `SeedAdmin` is a `Store` method combining two ordering-sensitive `Bootstrap` calls. This deviates from that precedent because the seed catalog is expected to grow, and coupling every future step to `Store`'s mutex — sized for `Mutate`'s async/`Bootstrap`'s sync dual contract — adds a dependency the steps don't need. Scope stays to `*appconfig.Config` only: nothing else is seeded today, so a document-agnostic module would be speculative. `Store.Bootstrap` remains the underlying persistence primitive each `Step` runs through; `Store` gains no new exported methods beyond the existing `SeedAdmin`, which the new package calls as one of its steps.

## Considered options

- **Disk-editable defaults file** (not embedded): rejected. Built-in defaults are meant to version with a release — `MergeMissingSidecarTypes` exists specifically so a new built-in ships in a future version and appears on the next restart. A runtime-editable file would let someone hand-edit built-ins with no audit trail, a side door around the one-path-through-the-config-store discipline ADR-0001 established for the live document.
- **Keep defaults as Go literals, JSON file only for step-local data**: rejected. Two sources of "the default" is exactly the class of bug already found (`ParallelCount`); one file backing both `bootstrap` and `appconfig.Default()` closes it.

## Consequences

- `main.go` shrinks to one call into `bootstrap.Run`, driven by a returned report for the existing one-time console printing and `http-client.private.env.json` sync.
- Admin's password/salt/hash generation is untouched — only `Username`/`Email` move into the defaults file; a credential hash can't be `{guid}`-templated.
- A step whose JSON section fails to unmarshal, or a `{guid}` substitution that can't be satisfied, must fail startup rather than seed a partial document — there is no later listener pass to catch a bad bootstrap write.
