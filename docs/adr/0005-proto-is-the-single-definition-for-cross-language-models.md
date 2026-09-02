---
status: accepted
---

# Proto is the single definition for any model that crosses a language boundary

Every model that crossed a language boundary in Metarr used to be written out by hand up to four times: a Go struct, a
proto message, the conversion glue between them, and a TypeScript type. Adding one field to the application config meant
four coordinated edits, and nothing failed if only three of them happened — the fourth drifted silently until someone
noticed a setting that never reached the UI, or a field the UI sent that the server ignored. Two files existed for no
purpose other than being those mirrors, and neither had any way to know when the thing it mirrored had changed.

## Decision

Proto is the single definition for any model that crosses a language boundary — the Go server, the TypeScript UI, and
the stored Mongo document. Go and TypeScript both read generated code. No hand-written mirror of a generated model may
exist in either language, and no conversion layer between a hand-written Go struct and its generated equivalent may
exist.

Concretely, per model family:

- the messages and any closed vocabularies are defined in proto and the existing `go generate` step produces both
  languages' code;
- each hand-written Go model struct becomes a type alias to its generated equivalent, kept in the package that owns that
  family so the rest of the codebase still names the type through that package. Behaviour that hung off those types as
  methods becomes package-level functions;
- the generated `Get*` accessors are not used — reads use plain field access, because the system is greenfield and a
  section written by current code is always present;
- the family's hand-written TypeScript mirror is deleted and its importers move to the generated type;
- the family's conversion functions are deleted.

An architecture test enforces the rule, so the next hand-written mirror fails the build rather than accumulating.

One narrow exception to "used directly as the stored document" is carved out below for `Agent`'s output-only
live-presence fields (ADR-0010). It is a read-path join the model layer owns, not a mirror, so the architecture test's
intent is unchanged.

## Costs accepted

**The admin credential fields are on the wire but always empty.** `AdminUser` is now both the wire shape and the stored
shape — the document Mongo holds — so `password_salt` and `password_hash` are fields on the message and Mongo can carry
them. A read of the application config blanks both before responding, and `UpdateAdmin` remains the only write path for
a new password (ADR 0001), so a generated client sees two fields that are always empty. This is the deliberate price of
one definition of the message rather than a wire copy and a stored copy. The blanking is done on a clone: live config
holds the running server's own credentials, and blanking them in place would lock the administrator out until the next
reload.

**Generated messages are used directly as stored documents.** The application config document is encoded with protojson
using proto field names, so stored field names stay snake_case and the document is still readable directly in the
collection, and with unpopulated fields emitted, so the document lists every setting rather than only the ones that
differ from zero. The singleton `_id` the document is stored under is a storage concern and is not a field on the
message. One consequence: proto3 draws no distinction between an absent repeated field and an empty one, so a bootstrap
step that normalised a nil slice to `[]` no longer has anything durable to do.

**`Agent` live-presence fields are not stored.** ADR-0010 folds live presence (`identity`, `telemetry`, `online`,
`reported_at`) onto the one `Agent` message as `OUTPUT_ONLY` fields. Presence is a running-server fact, not config, so
it is never persisted: the `AgentService` read path joins it in from the presence source, and the write path builds the
stored `Agent` from the operator fields only — a request that echoes presence back has those fields ignored. This is a
read-path join, not a hand-maintained mirror, so the rule's intent holds. ADR-0010's earlier draft also put an AIP
resource-name `name` and an AIP-154 `etag` on every resource, which would have needed a general derived-field-stripping
step on write; its final form drops both, so no such step exists and `Normalize()` keeps only its nil-section filling.

## Considered and rejected

- **Keep a hand-written Go struct and generate only TypeScript.** Smaller commits, but two edit points remain — which is
  the problem this work exists to remove.
- **A typed per-node-type settings union for the workflow graph.** That is the shape the workflow engine design warns
  silently drops user work: a node whose type a build does not recognise would lose its settings on save. The graph
  carries its open content as structured values instead.

## Out of scope

Migration and back-compatibility (the system is greenfield); the TypeScript port of the workflow type system's subtyping
and coercion rules, which is duplicated _behaviour_, not a duplicated model, and which code generation does not address;
and the Swagger surface, whose removal is a separate decision.
