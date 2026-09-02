---
status: accepted
---

# CRUD API shape follows AIP standard methods

The config API's original CRUD rule — "one upserting POST, an empty id
creates, an unknown id is 404, ids are server-minted" — was written when the
server was a REST `http.ServeMux`. The API is now Connect/gRPC generated from
proto by Buf, and gRPC's only widely-followed CRUD standard is Google's AIP
(API Improvement Proposals). We adopt the part of AIP that makes an API
predictable to someone who knows it — the standard methods and their names,
resource-shaped request/response messages, field-mask partial update, and the
full `List` contract — and deliberately leave the parts whose machinery this
system's scale and shape do not earn: resource-name addressing (AIP-122),
long-running operations (AIP-151), and resource `etag`s (AIP-154).

An earlier draft of this ADR adopted AIP whole, including those three. It was
reversed. The config API has one first-party client (the UI), one writer (the
single server process, ADR-0002), and operator-bounded collections. Against
that, resource names are a backfill/strip round-trip on every message with no
consumer that reads them; operations model an asynchronous write ADR-0002 no
longer performs; and an `etag` guards a lost update the single-writer lock and
the single admin account make theoretical. Each is a mechanism a contributor
has to learn for no behaviour it buys here. What stays is the shape, which is
where the predictability always was.

## Decision

**Standard methods, standard names.** Every config collection exposes
`Create`, `Get`, `List`, `Update`, `Delete` (AIP-131–135), each named for its
resource: `CreateAgent`, `ListSonarrInstances`, `GetScanDirectory`,
`UpdateSidecarType`, `DeleteApiKey`. No `Upsert` verb. No bare `Create` /
`List` even where a service hosts a single collection — one naming rule
everywhere.

**Full read surface.** Every collection has `Get` and `List`, including
`GetApiKey` / `ListApiKeys` and `GetAgent`, even though the aggregate
`ConfigService.GetConfig` read (below) still serves the UI's first paint.

**Identity is the slug or the minted id it already has.** No synthetic
`name`. The two idioms from CONTEXT.md are unchanged:

- *Slug-addressed* (`agents`, Sonarr instances, `scan_directories`): the
  operator-chosen slug is the id. `Get` / `Delete` take `string slug`;
  `Create{X}Request { string {x}_id; X {x} }` carries it in `{x}_id`
  (AIP-133), and a slug in the resource body must match it or be empty, or
  `InvalidArgument`. `Create` against an existing slug is `AlreadyExists`.
- *Minted-id* (`sidecar_types`, `api_keys`): `Create{X}Request` carries no
  id; the server mints one and returns the created resource with it set.

The slug stays the cross-resource link everywhere else it is used (agent
directory mappings, bus channel names) as a bare value — no `agents/` prefix,
no resource-reference annotation.

**Partial update is a field mask.** `Update{X}Request` carries a
`google.protobuf.FieldMask update_mask` (AIP-134), authoritative for which
fields change, replacing the proto3 `optional`-presence idiom. Dotted paths
are honoured (`storage.ttl`). An empty mask, or a path that names no field of
the resource, is `InvalidArgument`. The mask is applied with
`go.einride.tech/aip/fieldmask` over protobuf reflection — no per-field copy
code. `Update{X}Request` also carries `bool allow_missing`, slug-addressed
collections only: `true` means an `Update` against an unknown slug creates,
and on that branch the mask is ignored and the whole resource message is
validated as a `Create`. Minted-id `Update{X}Request` has no `allow_missing`
field — a knob that must always be false is a footgun.

**Not-found behaviour:**

| operation | unknown key → |
| --- | --- |
| slug `Update` (`allow_missing:true`) | creates |
| slug `Update` (`allow_missing:false`) | `NotFound` |
| slug `Create` (slug already exists) | `AlreadyExists` |
| minted-id `Update` | `NotFound` |
| `Delete`, either kind | `NotFound` |

**Writes are synchronous and return the resource.** `Create` / `Update`
return the stored resource; `Delete` returns empty. The config store persists
to MongoDB under its lock before the RPC returns (ADR-0002, amended in the
same change): there is no `google.longrunning.Operation`, no
`AcceptedResponse`, no `OperationsService`. Validation — `AlreadyExists`,
`NotFound`, `InvalidArgument` (bad mask, slug/body mismatch, cross-entry
failure), `FailedPrecondition` — surfaces as a Connect code on the call. A
persistence failure is a synchronous `Internal` on the same call; live config
is left untouched when a write does not land.

**Cross-entry validation runs on the write path**, against the whole
in-memory config before the store writes: the sidecar-registry compile, the
Sonarr cross-type slug-uniqueness check, `validateMappings`. Its failures map
to `InvalidArgument` / `FailedPrecondition`.

**`List` is paginated, filterable, and orderable.** `List{X}Request` carries
`int32 page_size` / `string page_token` (AIP-158), `string filter` (AIP-160),
and `string order_by` (AIP-132); `List{X}Response` carries the repeated
resource and `string next_page_token`. Pagination and ordering are wired now
with `go.einride.tech/aip`'s `pagination` and `ordering` packages over the
in-memory collection; `filter` is parsed and validated with
`go.einride.tech/aip/filtering` but only a documented subset is honoured until
a large-data service (scan records, metadata) needs the full
expression-to-storage translation, written then against data that requires
it. The config collections are bounded and a default page returns the whole
set — the request/response shape is the standard one, so a collection that
later grows unbounded needs no API change.

**Non-CRUD operations are custom methods** (AIP-136): `ReorderSidecarTypes`,
`ResetSidecarTypes`, `SetLogLevel` stay as named RPCs — they do not fit a
standard method. Each is synchronous and returns the affected resource or an
empty response. `SetLogLevel` is kept even though `UpdateAgent` with
`update_mask=["log_level"]` would do the same: a cheap dedicated method with a
distinct audit signal and its own caller.

**Agent presence rides one resource.** `AgentConfig` and `AgentView` collapse
into one `Agent` message. The operator fields (`slug`, `display_name`,
`mappings`, `log_level`) are writable; `identity`, `telemetry`, `online`,
`reported_at` are `OUTPUT_ONLY` — populated by `Get` / `List` /
`StreamPresence`, never copied from a request into the stored document. The
`AgentView` read type and the `agentregistry` view-alias layer are removed.

**Service decomposition.** `admin` becomes `AdminService` (`GetAdminUser`,
`UpdateAdminUser`); the API-key collection becomes `ApiKeyService` (`Create` /
`Get` / `List` / `Update` / `Delete`); each existing section keeps its own
service. `ConfigService` is left with one method, the read-only aggregate
`GetConfig` that returns the whole `Config` for the UI's first paint — no
matching write, so ADR-0001 is untouched. `password_salt` / `password_hash`
stay off the wire (ADR-0005); a new password travels in a separate
`string new_password` on `UpdateAdminUserRequest`, never in the mask, honoured
only when non-empty.

**API-key access level is an enum.** Requests that address the API-key
collection carry `AccessLevel access_level` (`ADMIN`, `USER`, `WEBHOOK`,
`READ_ONLY`) — a fixed four-value set, which AIP itself says should not be
modelled as a resource collection, so there is no `parent` addressing and no
`AccessLevelService`.

## What is adopted, and what is not

| AIP | area | decision |
| --- | --- | --- |
| 131–135 | resource-oriented standard methods | adopted — standard set, per-resource names |
| 133 | `Create` id in `{x}_id` | adopted — slug-addressed collections |
| 134 | field-mask partial update | adopted — `go.einride.tech/aip/fieldmask` |
| 136 | custom methods | adopted — reorder / reset / set-log-level |
| 158 | pagination | adopted — `go.einride.tech/aip/pagination` |
| 132 | ordering | adopted — `go.einride.tech/aip/ordering` |
| 160 | filtering | shape adopted; `go.einride.tech/aip/filtering` parses it, full translation deferred |
| 122 | resource-name addressing | **not adopted** — one client, nothing reads a `name`; the slug/id it already has is the identifier |
| 151 | long-running operations | **not adopted** — the write is synchronous (ADR-0002), so there is no operation to poll |
| 154 | resource `etag` | **not adopted** — the single-writer lock serialises writes and there is one admin account |

## Relationship to ADR-0001

Unchanged and reinforced: no whole-document write; every method names exactly
what it changes. The aggregate `GetConfig` is read-only. The old rule's "a
non-empty unknown id is rejected" special case disappears — it existed only to
disambiguate one message that meant both create and update, and separate
`Create` / `Update` methods remove the ambiguity.

## Relationship to ADR-0002

Amended in the same change. The config store's write becomes synchronous: it
persists to MongoDB under its lock and then propagates in-process, before the
RPC returns. The `system_config_update` Redis stream, its listener, and the
"queued" acknowledgement are removed. The lock is unchanged.

## Relationship to ADR-0005

No derived `name` or `etag` field is added to any message, so there is no
`ClearDerived` step and `Normalize()` keeps only its nil-section filling — the
message stays exactly the stored document. The one read-path addition is
`Agent`'s `OUTPUT_ONLY` presence, which the `AgentService` read path joins in
and which the write path never copies into the stored document.

## Considered options

- **Keep one `Upsert` per section.** Rejected: not a standard method, and it
  keeps the merged create-or-update message whose sometimes-empty id field is
  a `oneof` in disguise.
- **Adopt AIP whole — resource names, operations, and `etag`** (this ADR's
  previous draft). Rejected: at one first-party client, one writer, and
  operator-bounded collections, each of the three is documented machinery a
  contributor must learn for no behaviour it enables. Resource names cost a
  backfill/strip round-trip on every message; operations model an async write
  ADR-0002 no longer does; `etag` guards a race the lock and the single admin
  account make theoretical.
- **Keep proto3 `optional` field presence instead of `FieldMask`.** Rejected:
  presence and a mask do the same job and the mask is the standard.
- **A synthetic `name` for cross-resource references.** Rejected: the slug
  already links things, and prefixing it buys nothing without generic
  reference-walking tooling, which this system does not have.

## Consequences

- Roughly thirty standard methods across the config services — `AdminService`,
  `ApiKeyService`, and one per existing section — replacing the `Upsert{X}`
  RPCs and the `optional`-presence updates. **One greenfield sweep**, not
  staged: the config document is reinitialised, there is no migration, and
  staging would leave the UI and services disagreeing mid-branch.
- **Generated code only.** Message types, Connect interfaces, client stubs,
  and TS types come from `buf generate` / `go generate` (ADR-0005). Field
  masks, pagination, and ordering are `go.einride.tech/aip`; `resourcename` is
  not used.
- **No operations store and no config-update stream** — see ADR-0002. The
  `Operation` message, `OperationsService`, the `config_operations` Mongo
  collection, and `AcceptedResponse` (with its `buf.yaml`
  `RPC_REQUEST_RESPONSE_UNIQUE` / `RPC_RESPONSE_STANDARD_NAME` suppressions)
  are all removed.
- **The UI updates its cache from the write's response** instead of polling a
  queued→confirmed indicator; the `useConfirmationPoll` / `useSaveState`
  re-read loop is removed.
- `AGENTS.md`'s CRUD-API rule and the `config-structure-change` skill are
  updated in the same change: a new config collection is `Create` + `Get` +
  `List` (paginated) + `Update` + `Delete` addressed by its slug or minted
  id, with a `FieldMask` on `Update`; a new scalar section is `Get` + `Update`
  with a `FieldMask`. Writes are synchronous and return the resource.
- `documentation/modules/ROOT/pages/configuration.adoc` and the "eventually
  consistent" section of `architecture.adoc` are de-`Upsert`ed and matched to
  the synchronous shape. `documentation/modules/design/` needs no change.
- `builtin_defaults.json` and the `bootstrap` package are unchanged: storage
  is still one `app_config` document keyed on slug / minted id (ADR-0011),
  with no `name` or `etag` field.
