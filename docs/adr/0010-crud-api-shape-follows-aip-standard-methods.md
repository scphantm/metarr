---
status: accepted
---

# CRUD API shape follows AIP standard methods

The config API's CRUD rule — "one upserting POST, an empty id creates, an unknown
id is 404, ids are server-minted" — was written when the server was a REST
`http.ServeMux`. The API is now Connect/gRPC generated from proto by Buf, and
gRPC's only widely-followed CRUD standard is Google's AIP (API Improvement
Proposals). We adopt AIP whole: the standard methods, resource-oriented
addressing, field-mask partial update, `etag` optimistic concurrency,
long-running operations for the async write, and paginated/filterable/orderable
`List`.

Two earlier drafts each took a narrower line. The first kept standard method
names only — bare id/slug request fields, no resource names, no pagination. The
second added resource names and field masks but still carved out `etag` and
`google.longrunning.Operation` as "deliberate deviations" the event-sourced
store forced. Both narrow lines are the same maintenance surprise: a reader who
knows AIP has to re-derive which half of it applies here, and every "deviation"
paragraph is a place the doc and a contributor's expectation drift apart.
Committing to AIP means committing to all of it. The mechanical cost of the
parts we kept deferring is bounded by `go.einride.tech/aip`, which implements
the resource-name, field-mask, pagination, filtering, and ordering plumbing so
none of it is hand-rolled.

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
standalone access-level enum that requests carried before is gone. Names are
parsed and formatted with `go.einride.tech/aip/resourcename`, not a hand-written
splitter.

**Partial update is a field mask.** `Update{X}Request` carries a
`google.protobuf.FieldMask update_mask` (AIP-134), authoritative for which
fields change, replacing the proto3 `optional`-presence patch idiom. Dotted
paths are honoured (`storage.ttl`). An empty mask, or a path that names no
field of the resource, is `InvalidArgument`. The mask is applied with
`go.einride.tech/aip/fieldmask` over `google.golang.org/protobuf` reflection —
no bespoke per-field copy code. When a slug `Update` with `allow_missing:true`
creates instead of updating, the mask is ignored and the full resource message
is validated as a `Create`.

**Upsert semantics are `allow_missing` on `Update`, slug-addressed only.**
Slug-addressed `Update{X}Request` carries `bool allow_missing`; `true` means an
`Update` against an unknown slug creates. Minted-id `Update{X}Request` has **no**
`allow_missing` field — a knob that must always be false is a footgun, and its
absence is the "no upsert here" semantics.

**Optimistic concurrency with `etag`** (AIP-154). Every resource and every
scalar section carries `string etag`, `google.api.field_behavior = OUTPUT_ONLY`.
The etag is a hash of that section's stored bytes — derived on read, never
itself stored (ADR-0005). `Update` and `Delete` carry the etag the client last
read; the config-store mutation closure recomputes the current section's hash
under the store lock and returns `ABORTED` if it differs. An empty etag on the
request skips the check (a deliberate blind write). This closes the lost-update
window the single-writer lock alone leaves open: the lock orders one request's
read after the previous request's fire, but a client editing from a copy it read
minutes ago is not otherwise protected. See ADR-0002.

**Identity still has two idioms** (CONTEXT.md, unchanged):

- *Slug-addressed* (`agents`, `sonarr`, `scan_directories`): the operator-chosen
  slug is the id. On `Create` it travels in `Create{X}Request.{x}_id` (AIP-133),
  not inside the resource message; if the resource body also carries its slug
  field it must match, or `InvalidArgument`. `Create` against an existing slug
  is `AlreadyExists`.
- *Minted-id* (`sidecar_types`, `api_keys`): `Create{X}Request` carries no id;
  the server mints one. The client learns it from the operation's `response`
  once the write confirms.

**Not-found behaviour:**

| operation | unknown key → |
| --- | --- |
| slug `Update` (`allow_missing:true`) | creates |
| slug `Update` (`allow_missing:false`) | `NotFound` |
| slug `Create` (slug already exists) | `AlreadyExists` |
| minted-id `Update` | `NotFound` |
| `Delete`, either kind | `NotFound` |

**Writes return a long-running operation** (AIP-151). `Create`, `Update`,
`Delete` and the config-mutating custom methods return a
`google.longrunning.Operation`, not the resource. The config store is
event-sourced and eventually consistent (ADR-0002): the durable write happens in
the `system_config_update` listener after the RPC returns. The operation's
`name` is `operations/{correlation_id}`, reusing the correlation id the RPC
already stamps on its fired event. A new `OperationsService` exposes
`GetOperation` (and `ListOperations`); the listener marks the operation `done`
once it has persisted, setting `response` to the resource on success or `error`
to a `google.rpc.Status` on a persistence or late-validation failure. The UI
polls `GetOperation` in place of the old re-read-and-confirm loop, and a
persistence failure now reaches the caller — which the bare "queued" acknowledgement
could not deliver.

**Synchronous vs. deferred errors.** The config-store mutation closure runs
inside the RPC, under the store lock, before the operation is returned.
`AlreadyExists`, `NotFound`, `InvalidArgument` (bad mask, slug/body mismatch,
cross-entry validation failure), and `ABORTED` (stale etag) surface as Connect
codes on the call itself. Only durable persistence is deferred, and its failure
surfaces as `operation.error`.

**Cross-entry validation runs inside the mutation closure**, against the whole
section, so a scoped write is still checked against the whole table (the
sidecar-registry compile, the Sonarr cross-type slug-uniqueness check,
`validateMappings`). Its failures map to `InvalidArgument` / `FailedPrecondition`.

**`List` is paginated, filterable, and orderable.** `List{X}Request` carries
`int32 page_size` / `string page_token` (AIP-158), `string filter` (AIP-160),
and `string order_by` (AIP-132); `List{X}Response` carries the repeated resource
and `string next_page_token`. Implemented with `go.einride.tech/aip`'s
`pagination`, `filtering`, and `ordering` packages. Today's collections are
bounded and a default page returns the whole set, but the request/response shape
is the standard one, so a collection that later grows unbounded needs no API
change.

**Non-CRUD operations are custom methods** (AIP-136): `ReorderSidecarTypes`,
`ResetSidecarTypes`, `SetLogLevel` stay as named RPCs — they do not fit a
standard method. `SetLogLevel` is kept even though `UpdateAgent` with
`update_mask=["log_level"]` would do the same: it is a cheap dedicated method
with a distinct audit signal and its own caller. Each still returns a
long-running operation.

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

## AIP compliance

Every AIP area the config API touches is adopted rather than carved out:

| AIP | area | how |
| --- | --- | --- |
| 121–135 | resource-oriented standard methods | standard method set, per-resource names |
| 122 | resource names | `go.einride.tech/aip/resourcename` |
| 133 | `Create` id in `{x}_id` | slug-addressed collections only |
| 134 | field-mask partial update | `go.einride.tech/aip/fieldmask` |
| 136 | custom methods | reorder / reset / set-log-level |
| 151 | long-running operations | `OperationsService`, listener completes them |
| 154 | resource `etag` | hash of stored bytes, checked under the store lock |
| 158 | pagination | `go.einride.tech/aip/pagination` |
| 160 | filtering | `go.einride.tech/aip/filtering` |
| 132 | ordering | `go.einride.tech/aip/ordering` |

## Relationship to ADR-0001

Unchanged and reinforced: still no whole-document write; every method names
exactly what it changes. ADR-0001 established "scoped operations"; this ADR is
the concrete method shapes that principle takes under gRPC. The old rule's "a
non-empty unknown id is rejected" special case disappears — it existed only to
disambiguate a single message that meant both create and update, and separate
`Create` / `Update` methods remove the ambiguity.

## Relationship to ADR-0002

ADR-0002 is amended in the same change. The single-instance lock and the
fire-and-return write are unchanged; what changes is how the async write is
surfaced (a `google.longrunning.Operation` the caller polls, instead of a bare
"queued" acknowledgement) and that `etag` now guards the lost-update case the
lock does not cover.

## Relationship to ADR-0005

Resource-name addressing puts a `name` on every resource, presence puts
`OUTPUT_ONLY` fields on `Agent`, and `etag` puts an `OUTPUT_ONLY` concurrency
token on every resource and scalar section. None is authoritative in storage:
`name` is derived from the slug or minted id, presence is joined in by the read
path, `etag` is a hash of the section's stored bytes. The mutation closure
clears all three before `MarshalStored`; `Normalize()` backfills `name` and
`etag` on read. ADR-0005's "generated messages are used directly as stored
documents" clause is amended in the same change to permit these symmetric,
lossless transforms — they are not a hand-maintained mirror.

## Considered options

- **Keep one `Upsert` per section.** Rejected: not a standard method, and it
  keeps the merged create-or-update message whose sometimes-empty id field is a
  `oneof` in disguise — the 404-on-unknown-id clause is the wart that proves it.
- **Standard method names only, no resource names or pagination** (this ADR's
  first draft). Rejected: half of AIP is its own surprise.
- **Resource names and field masks, but `AcceptedResponse` and no `etag`**
  (this ADR's second draft). Rejected on the same ground one draft up: a
  documented deviation is a re-derivation cost, and the two we kept were the
  ones a contributor most expects AIP to have. `go.einride.tech/aip` and a
  `google.longrunning.Operation` whose name is the existing correlation id make
  the mechanical cost small.
- **Bare `Create` / `List` / `Get` on single-collection services.** Rejected: a
  third naming convention alongside "standard" and "custom".
- **Keep proto3 `optional` field presence instead of `FieldMask`.** Rejected:
  presence and a mask do the same job and the mask is the standard.
- **A stored `generation` counter for `etag` instead of a content hash.**
  Rejected: a hash needs nothing added to the stored document and keeps ADR-0005's
  "nothing derived is stored" carve-out intact; a counter would be a new
  authoritative stored field and a compare-and-swap on it.

## Consequences

- Roughly thirty standard methods across eight services (the six existing config
  services, the new `AdminService`, and the new `OperationsService`), replacing
  five `Upsert{X}` RPCs and the `optional`-presence updates. **One greenfield
  sweep**, not staged per service: the no-migration, config-document-reinitialised
  premise removes the reason to stage, and staging would leave the UI and
  services disagreeing mid-branch.
- **Generated code only.** Message types, Connect service interfaces, client
  stubs, and TS types come from `buf generate` / `go generate` (ADR-0005) — no
  hand-authored generated artifacts. Field masks, resource names, pagination,
  filtering, and ordering are `go.einride.tech/aip`, not bespoke code. The
  hand-rolled `internal/shared/aip` helper from the earlier groundwork is
  removed; the only Metarr-specific piece that stays is `ClearDerived` (the
  storage carve-out), which moves to `internal/shared/appconfig`.
- **A server-only operations store.** `OperationsService` is backed by a Mongo
  collection the server owns (agents never see it); the `system_config_update`
  listener gains a step that resolves the operation by correlation id and marks
  it done. `google/longrunning/operations.proto` and `google/rpc/status.proto`
  come from the `buf.build/googleapis/googleapis` dependency already added for
  `google/api/field_behavior.proto`. `google/protobuf/field_mask.proto` is a
  well-known type — imported, no dep.
- **The UI polls `GetOperation`.** Every config mutation hook changes from
  "fire, then re-read the resource until it matches" to "fire, then poll the
  returned operation until `done`, then surface `error` or invalidate the
  reads". Every resource read surfaces `etag`; every `Update` / `Delete` sends
  the last-read etag back.
- `SidecarTypeDefinition` gains a `string name` field in the frozen
  `metarr.bus.v1` module. A field addition is wire-compatible and passes the
  FILE-level `buf breaking` gate; the message already does double duty as a
  stored document and an agent-projection type.
- `AGENTS.md`'s CRUD-API rule and the `config-structure-change` skill are
  updated in the same change: a new config collection is `Create` + `Get` +
  `List` (paginated / filterable) + `Update` + `Delete` with `name`/`parent`
  addressing, a `FieldMask` on `Update`, and an `etag`; a new scalar section is
  `Get` + `Update` with a `FieldMask` and an `etag`. Writes return an operation.
- `documentation/modules/ROOT/pages/configuration.adoc` and the "eventually
  consistent" section of `architecture.adoc` are de-`Upsert`ed and matched to the
  new shape alongside the code slices. `documentation/modules/design/` needs no
  change — its config references are shape-agnostic.
- `builtin_defaults.json` and the `bootstrap` package are unchanged: the stored
  document keeps its current shape, keyed on slug / minted id, with no `name`,
  `etag`, or presence field (`Normalize()` backfills `name` and `etag` on load).
