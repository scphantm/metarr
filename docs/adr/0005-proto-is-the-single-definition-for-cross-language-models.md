---
status: accepted
---

# Proto is the single definition for any model that crosses a language boundary

Every model that crossed a language boundary in Metarr used to be written out
by hand up to four times: a Go struct, a proto message, the conversion glue
between them, and a TypeScript type. Adding one field to the application config
meant four coordinated edits, and nothing failed if only three of them
happened — the fourth drifted silently until someone noticed a setting that
never reached the UI, or a field the UI sent that the server ignored. Two files
existed for no purpose other than being those mirrors, and neither had any way
to know when the thing it mirrored had changed.

## Decision

Proto is the single definition for any model that crosses a language boundary —
the Go server, the TypeScript UI, and the stored Mongo document. Go and
TypeScript both read generated code. No hand-written mirror of a generated
model may exist in either language, and no conversion layer between a
hand-written Go struct and its generated equivalent may exist.

Concretely, per model family:

- the messages and any closed vocabularies are defined in proto and the
  existing `go generate` step produces both languages' code;
- each hand-written Go model struct becomes a type alias to its generated
  equivalent, kept in the package that owns that family so the rest of the
  codebase still names the type through that package. Behaviour that hung off
  those types as methods becomes package-level functions;
- the generated `Get*` accessors are not used — reads use plain field access,
  because the system is greenfield and a section written by current code is
  always present;
- the family's hand-written TypeScript mirror is deleted and its importers
  move to the generated type;
- the family's conversion functions are deleted.

An architecture test enforces the rule, so the next hand-written mirror fails
the build rather than accumulating.

One narrow exception to "used directly as the stored document" is carved out
below for AIP-derived identifier, concurrency, and output-only fields
(ADR-0010) — the resource `name`, the `etag`, and `Agent` presence. Each is a
symmetric, lossless transform the model layer owns, not a mirror, so the
architecture test's intent is unchanged.

## Costs accepted

**The admin credential fields are on the wire but always empty.** `AdminUser`
is now both the wire shape and the stored shape — the document Mongo holds —
so `password_salt` and `password_hash` are fields on the message and Mongo can
carry them. A read of the application config blanks both before responding, and
`UpdateAdmin` remains the only write path for a new password (ADR 0001), so a
generated client sees two fields that are always empty. This is the deliberate
price of one definition of the message rather than a wire copy and a stored
copy. The blanking is done on a clone: live config holds the running server's
own credentials, and blanking them in place would lock the administrator out
until the next reload.

**Generated messages are used directly as stored documents.** The application
config document is encoded with protojson using proto field names, so stored
field names stay snake_case and the document is still readable directly in the
collection, and with unpopulated fields emitted, so the document lists every
setting rather than only the ones that differ from zero. The singleton `_id`
the document is stored under is a storage concern and is not a field on the
message. One consequence: proto3 draws no distinction between an absent
repeated field and an empty one, so a bootstrap step that normalised a nil
slice to `[]` no longer has anything durable to do.

**AIP identifier, concurrency, and output-only fields are not stored.** When
ADR-0010 put an AIP resource-name `name` and an AIP-154 `etag` on every config
resource and output-only live-presence fields on `Agent`, storing those
verbatim would be either redundant (`name` is derived from the slug or minted id
the document is already keyed on; `etag` is a hash of the section's own stored
bytes) or wrong (presence is a running-server fact, not config). So they are the
one exception to "used directly as the stored document": `MarshalStored` calls
`appconfig.ClearDerived` on a clone before it serializes — the one chokepoint
every persist and every `system_config_update` payload passes through, so no
caller can leak a derived field — `Normalize()` backfills `name` and `etag` on
read, and the `AgentService` read path joins presence in. The stored document
keeps exactly the shape it had before ADR-0010. This stays within the rule
because the transform is symmetric and total — every value is recomputed from
data already in the document or from live state — so there is nothing a person
keeps aligned by hand and nothing that can silently drift.

## Considered and rejected

- **Keep a hand-written Go struct and generate only TypeScript.** Smaller
  commits, but two edit points remain — which is the problem this work exists
  to remove.
- **A typed per-node-type settings union for the workflow graph.** That is the
  shape the workflow engine design warns silently drops user work: a node whose
  type a build does not recognise would lose its settings on save. The graph
  carries its open content as structured values instead.

## Out of scope

Migration and back-compatibility (the system is greenfield); the TypeScript
port of the workflow type system's subtyping and coercion rules, which is
duplicated *behaviour*, not a duplicated model, and which code generation does
not address; and the Swagger surface, whose removal is a separate decision.
