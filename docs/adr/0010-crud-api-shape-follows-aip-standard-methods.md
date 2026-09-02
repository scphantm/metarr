---
status: accepted
---

# CRUD API shape follows AIP standard methods

The config API's CRUD rule — "one upserting POST, an empty id creates, an unknown
id is 404, ids are server-minted" — was written when the server was a REST
`http.ServeMux`. The API is now Connect/gRPC generated from proto by Buf, and
gRPC's only widely-followed CRUD standard is Google's AIP (API Improvement
Proposals). We are adopting the AIP standard methods, adapted where Metarr's
event-sourced config store makes the literal AIP contract impossible.

## Decision

**Standard methods, standard names.** Each config collection exposes `Create`,
`Get`, `List`, `Update`, `Delete` (AIP-131–135). There is no `Upsert` verb.

**Partial update is a field mask.** `Update{X}Request` carries a
`google.protobuf.FieldMask update_mask` (AIP-134), authoritative for which fields
change. It replaces the proto3 `optional`-presence patch idiom the scalar
sections use today.

**Upsert semantics are `allow_missing` on `Update`.** `Update{X}Request` carries
`bool allow_missing`. Slug-addressed sections send `true` — an `Update` against an
unknown slug creates. Minted-id sections leave it `false` — an `Update` against an
unknown id is `NotFound`.

**Identity still has two idioms** (CONTEXT.md, unchanged):

- *Slug-addressed* (`agents`, `sonarr`, `scan_directories`): the operator-chosen
  slug is the id. It travels in `Create{X}Request.{x}_id` (AIP-133), not inside
  the resource message. `Create` against an existing slug is `AlreadyExists`.
- *Minted-id* (`sidecar_types`, `api_keys`): `Create{X}Request` carries no id; the
  server mints one. The client learns it by re-reading once the write confirms.

**Not-found behaviour:**

| operation | unknown key → |
| --- | --- |
| slug `Update` (`allow_missing:true`) | creates |
| slug `Create` (slug already exists) | `AlreadyExists` |
| minted-id `Update` (`allow_missing:false`) | `NotFound` |
| `Delete`, either kind | `NotFound` |

**Non-CRUD operations are custom methods** (AIP-136): `ReorderSidecarTypes`,
`ResetSidecarTypes`, `SetLogLevel` stay as named RPCs — they do not fit a standard
method and are not forced into one.

**Cross-entry validation runs inside the config-store mutation closure**, against
the whole section, so a scoped write is still checked against the whole table
(the sidecar-registry compile, `validateMappings`).

## Deliberate deviations from AIP

- **Writes return `AcceptedResponse` (a correlation id), not the resource and not
  a `google.longrunning.Operation`.** Config writes are event-sourced and
  eventually consistent (ADR-0002): the resource does not exist yet when the RPC
  returns. The client polls queued→confirmed (`useConfirmationPoll`). Adopting
  `google.longrunning` would be a large surface — operation store, `GetOperation`,
  poll plumbing — for a single first-party UI that already polls.
- **No `etag` / optimistic concurrency (AIP-154).** The config store is
  single-writer behind a process-local lock (ADR-0002); an `etag` field would be
  decorative.
- **No resource-name string addressing (`agents/nas-01`) and no
  `page_size`/`page_token` pagination.** One first-party UI client, small bounded
  collections — the addressing and pagination ceremony buys nothing here.

## Relationship to ADR-0001

Unchanged and reinforced: still no whole-document write; every method names
exactly what it changes. ADR-0001 established "scoped operations"; this ADR is the
concrete method shapes that principle takes under gRPC. The old rule's "a
non-empty unknown id is rejected" special case disappears — it existed only to
disambiguate a single message that meant both create and update, and separate
`Create`/`Update` methods remove the ambiguity.

## Considered options

- **Keep one `Upsert` per section, just re-word "POST/PUT" as "one `Upsert`
  method".** Rejected: not a standard method, and it keeps the merged
  create-or-update message whose sometimes-empty id field is a `oneof` in
  disguise — the 404-on-unknown-id clause is the wart that proves it.
- **Full resource-oriented AIP** (resource names, pagination,
  `google.longrunning.Operation`, `etag`). Rejected: a large mechanical change to
  every message and handler that fights the async single-writer store, with no
  payoff for one UI client and bounded collections.
- **Keep proto3 `optional` field presence instead of `FieldMask` for partial
  update.** Rejected now that we are committing to AIP: presence and a mask do the
  same job and the mask is the standard.

## Consequences

- The five `Upsert{X}` RPCs split into `Create{X}` + `Update{X}`; roughly sixteen
  methods across five services are reshaped. Migration is per-service and
  opportunistic, **except `sidecar_types` and `api_keys`** — the minted-id
  sections that motivated this — which migrate first. No data migration; the
  config document is reinitialised.
- Proto doc comments are rewritten **per RPC as that RPC migrates**, never ahead
  of the code. Issue #15 (finding 3) was a doc comment describing behaviour the
  code did not have; this ADR is the target, not a description of today's
  handlers.
- `AGENTS.md`'s CRUD-API rule and the `config-structure-change` skill are updated
  in the same change as this ADR. A new config section is now "`Create` +
  `Update` (+ `List`/`Get`/`Delete`) following AIP", not "one upserting POST".
- The `documentation/modules/design` references to "the config CRUD API" are
  shape-agnostic and stay accurate; the transport-stale `POST /api/...` path
  strings in `workflow_engine.adoc` belong to a separate Connect-migration doc
  pass, not this one.
