---
status: accepted
---

# CRUD API shape follows AIP standard methods

The config API's CRUD rule — "one upserting POST, an empty id creates, an unknown
id is 404, ids are server-minted" — was written when the server was a REST
`http.ServeMux`. The API is now Connect/gRPC generated from proto by Buf, and
gRPC's only widely-followed CRUD standard is Google's AIP (API Improvement
Proposals). We adopt the AIP standard methods **and** resource-oriented
addressing, with two deviations that the event-sourced config store (ADR-0002)
forces.

An earlier draft of this ADR took a narrower line — standard method names only,
keeping bare id/slug request fields and skipping resource names and pagination.
It never shipped. The narrow line is its own maintenance surprise: a reader who
knows AIP has to re-derive which half of it applies here. Committing to AIP
means committing to its addressing model too.

## Decision

**Standard methods, standard names.** Every config collection exposes `Create`,
`Get`, `List`, `Update`, `Delete` (AIP-131–135), each named for its resource:
`CreateAgent`, `ListSonarrInstances`, `GetScanDirectory`, `UpdateSidecarType`,
`DeleteApiKey`. No `Upsert` verb. No bare `Create`/`List` even where the
service hosts a single collection — one naming rule, applied everywhere.

**Full read surface.** Every collection has `Get` and `List`, including
`ListApiKeys` / `GetApiKey` and `GetAgent`, even though the aggregate
`ConfigService.GetConfig` read already serves today's UI. Half a read surface
is the same re-derivation cost as half an addressing model.

**Resource-name addressing** (AIP-122). Every resource carries a `string name`:

| collection | pattern | parent |
| --- | --- | --- |
| agents | `agents/{slug}` | — (top-level) |
| sonarr instances | `sonarrInstances/{slug}` | — |
| scan directories | `scanDirectories/{slug}` | — |
| sidecar types | `sidecarTypes/{id}` | — |
| API key entries | `accessLevels/{level}/apiKeys/{id}` | `accessLevels/{level}` |

`Get` / `Update` / `Delete` address the resource by `name`. `List` / `Create`
take `parent` — empty for the four top-level collections, `accessLevels/{level}`
for API keys. `accessLevels/{level}` is an un-serviced parent path: the access
level is a fixed four-value set (`admin`, `user`, `webhook`, `read_only`), not a
managed collection, so there is no `AccessLevel` service and no
`ListAccessLevels`. The service parses `{level}` out of `parent` / `name`; the
standalone access-level enum that requests carried before is gone.

**Partial update is a field mask.** `Update{X}Request` carries a
`google.protobuf.FieldMask update_mask` (AIP-134), authoritative for which
fields change, replacing the proto3 `optional`-presence patch idiom. Dotted
paths are honoured (`storage.ttl`). An empty mask, or a path that names no
field of the resource, is `InvalidArgument`. When a slug `Update` with
`allow_missing:true` creates instead of updating, the mask is ignored and the
full resource message is validated as a `Create`.

**Upsert semantics are `allow_missing` on `Update`, slug-addressed only.**
Slug-addressed `Update{X}Request` carries `bool allow_missing`; `true` means an
`Update` against an unknown slug creates. Minted-id `Update{X}Request` has **no**
`allow_missing` field — a knob that must always be false is a footgun, and its
absence is the "no upsert here" semantics.

**Identity still has two idioms** (CONTEXT.md, unchanged):

- *Slug-addressed* (`agents`, `sonarr`, `scan_directories`): the operator-chosen
  slug is the id. On `Create` it travels in `Create{X}Request.{x}_id` (AIP-133),
  not inside the resource message; if the resource body also carries its slug
  field it must match, or `InvalidArgument`. `Create` against an existing slug
  is `AlreadyExists`.
- *Minted-id* (`sidecar_types`, `api_keys`): `Create{X}Request` carries no id;
  the server mints one. The client learns it by re-reading once the write
  confirms.

**Not-found behaviour:**

| operation | unknown key → |
| --- | --- |
| slug `Update` (`allow_missing:true`) | creates |
| slug `Update` (`allow_missing:false`) | `NotFound` |
| slug `Create` (slug already exists) | `AlreadyExists` |
| minted-id `Update` | `NotFound` |
| `Delete`, either kind | `NotFound` |

**Errors are synchronous; only persistence is async.** The config-store
mutation closure runs inside the RPC, under the store lock, before the RPC
returns. `AlreadyExists`, `NotFound`, `InvalidArgument` (bad mask, slug/body
mismatch, cross-entry validation failure) surface as Connect codes on the call.
Only durable persistence happens later, behind `AcceptedResponse` and the
`queued→confirmed` poll.

**Cross-entry validation runs inside the mutation closure**, against the whole
section, so a scoped write is still checked against the whole table (the
sidecar-registry compile, the Sonarr cross-type slug-uniqueness check,
`validateMappings`). Its failures map to `InvalidArgument` / `FailedPrecondition`.

**Non-CRUD operations are custom methods** (AIP-136): `ReorderSidecarTypes`,
`ResetSidecarTypes`, `SetLogLevel` stay as named RPCs — they do not fit a
standard method. `SetLogLevel` is kept even though `UpdateAgent` with
`update_mask=["log_level"]` would do the same: it is a cheap dedicated method
with a distinct audit signal and its own caller.

**Agent presence rides one resource.** `AgentService` returns the same `Agent`
message that `CreateAgent` / `UpdateAgent` accept — no separate read type. Live
presence fields on `Agent` are `google.api.field_behavior = OUTPUT_ONLY`:
populated by `Get` / `List` / `StreamPresence`, ignored by writes, never stored.
The `AgentView` read type is removed.

**A new `AdminService`.** `admin` becomes its own service — `GetAdminUser`,
`UpdateAdminUser` — rather than sitting on `ConfigService`. `password_salt` /
`password_hash` stay off the wire (ADR-0005); a new password travels in a
separate `string new_password` field, never in the mask, honoured only when
non-empty. `ConfigService` keeps the aggregate `GetConfig` read and the API-key
collection.

## Deliberate deviations from AIP

Both are downstream of ADR-0002's async single-writer model, not shape
preferences we could reverse cheaply.

- **Writes return `AcceptedResponse` (a correlation id), not the resource and
  not a `google.longrunning.Operation`.** Config writes are event-sourced and
  eventually consistent (ADR-0002): the resource does not exist yet when the RPC
  returns. The client polls `queued→confirmed` (`useConfirmationPoll`). Adopting
  `google.longrunning` would be a large surface — an operation store,
  `GetOperation`, an `Operations` service, poll plumbing rewired onto operation
  names — for one first-party UI that already polls. `Create` returns
  `AcceptedResponse` too; a minted-id `Create` is followed by a re-read.
- **No `etag` / optimistic concurrency (AIP-154).** The config store is
  single-writer behind a process-local lock (ADR-0002); concurrent-write
  conflict is impossible by construction. A meaningful `etag` would need a
  version or generation on the stored document and compare-and-swap semantics —
  exactly what the single-writer model was chosen to avoid.

Pagination (`page_size` / `page_token`) is also skipped, but as a plain
bounded-collection choice: `List` returns the full set, which is well-defined
for collections this small, and a page token with no backing cursor store would
be a fiction. If a collection ever grows unbounded, pagination is added then.

## Relationship to ADR-0001

Unchanged and reinforced: still no whole-document write; every method names
exactly what it changes. ADR-0001 established "scoped operations"; this ADR is
the concrete method shapes that principle takes under gRPC. The old rule's "a
non-empty unknown id is rejected" special case disappears — it existed only to
disambiguate a single message that meant both create and update, and separate
`Create` / `Update` methods remove the ambiguity.

## Relationship to ADR-0005

Resource-name addressing puts a `name` on every resource, and presence puts
`OUTPUT_ONLY` fields on `Agent`. Neither is authoritative in storage: `name` is
derived from the slug or minted id, presence is joined in by the read path. The
mutation closure clears both before `MarshalStored`; `Normalize()` backfills
`name` on read. ADR-0005's "generated messages are used directly as stored
documents" clause is amended in the same change to permit this one symmetric,
lossless transform — it is not a hand-maintained mirror.

## Considered options

- **Keep one `Upsert` per section.** Rejected: not a standard method, and it
  keeps the merged create-or-update message whose sometimes-empty id field is a
  `oneof` in disguise — the 404-on-unknown-id clause is the wart that proves it.
- **Standard method names only, no resource names or pagination** (this ADR's
  own first draft). Rejected: half of AIP is its own surprise. Resource-name
  addressing also makes API-key access-level nesting fall out cleanly as
  `parent`, instead of a bespoke enum field on every request.
- **Bare `Create` / `List` / `Get` on single-collection services**, resource
  suffixes only where a service hosts several. Rejected: a third naming
  convention alongside "standard" and "custom". The message names already carry
  the resource for uniqueness; the method names match them.
- **`google.longrunning.Operation` + `etag` for full compliance.** Rejected:
  fights the async single-writer store ADR-0002 established on its own merits;
  large mechanical change for one polling UI client.
- **Keep proto3 `optional` field presence instead of `FieldMask`.** Rejected:
  presence and a mask do the same job and the mask is the standard.

## Consequences

- Roughly thirty standard methods across seven services (the six existing config
  services plus the new `AdminService`), replacing five `Upsert{X}` RPCs and the
  `optional`-presence updates. **One greenfield sweep**, not staged per service:
  the no-migration, config-document-reinitialised premise removes the reason to
  stage, and staging would leave the UI and services disagreeing mid-branch.
- **Generated code only.** Message types, Connect service interfaces, client
  stubs, and TS types come from `buf generate` / `go generate` (ADR-0005) — no
  hand-authored generated artifacts. FieldMask is applied with
  `google.golang.org/protobuf` reflection and `fieldmaskpb`, not bespoke
  per-field copy code.
- `google/api/field_behavior.proto` is added as a `buf.build/googleapis/googleapis`
  dependency in `buf.yaml` (`buf dep update` writes `buf.lock`).
  `google/protobuf/field_mask.proto` is a well-known type — imported, no dep.
- `SidecarTypeDefinition` gains a `string name` field in the frozen
  `metarr.bus.v1` module. A field addition is wire-compatible and passes the
  FILE-level `buf breaking` gate; the message already does double duty as a
  stored document and an agent-projection type.
- `AGENTS.md`'s CRUD-API rule and the `config-structure-change` skill are
  updated in the same change: a new config collection is now `Create` + `Get` +
  `List` + `Update` + `Delete` with `name`/`parent` addressing and a
  `FieldMask` on `Update`; a new scalar section is `Get` + `Update` with a
  `FieldMask`.
- `documentation/modules/ROOT/pages/configuration.adoc` and the
  "eventually consistent" section of `architecture.adoc` are de-`Upsert`ed and
  matched to the new shape in the same change. `documentation/modules/design/`
  needs no change — its config references are shape-agnostic.
- `builtin_defaults.json` and the `bootstrap` package are unchanged: the stored
  document keeps its current shape, keyed on slug / minted id, with no `name`
  field (`Normalize()` backfills it on load).
