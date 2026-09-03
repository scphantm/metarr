---
status: accepted
---

# Deleting a workflow removes all of its versions

Issue #112 gives `WorkflowService` the full AIP standard-method set (ADR-0010), including a `DeleteWorkflow` the pre-AIP
`Upsert`-only shape never had. Workflows are stored in the append-only versioned collection
(`internal/server/mongostore/versioned`): every save inserts a brand-new immutable document and flips `is_latest`, so
history is never mutated in place. A delete has no obvious form in that model, so the choice is recorded here.

## Decision

`DeleteWorkflow(id)` **hard-removes every version of that workflow** — one `DeleteMany({document_id: id})` — and returns
`google.protobuf.Empty`. It is not a soft delete, not a tombstone version appended to the chain, and not a latest-only
removal. An id with no versions is `NotFound`, matching `Get` / `GetWorkflowVersion`.

The delete lives on `versioned.Store` as `DeleteAllVersions`, alongside the append-only writes, and the workflow repo
exposes it through the narrow `WorkflowStore` seam the service depends on.

## Why

- **A tombstone version breaks every reader.** `GetLatest`, `ListLatest`, and `ListVersions` would each need to learn to
  skip a deleted marker, and the versioned store is meant to be reused by future document types — teaching it a
  workflow-specific "deleted" state pollutes a generic component.
- **There is nothing to preserve a deleted workflow _for_.** No audit requirement, no undelete feature, no downstream
  reference. A workflow that no engine has run yet (the engine has not landed) and that the operator has chosen to
  remove is exactly as disposable as it looks.
- **It matches how the config API's `Delete` already behaves** — a real removal, `NotFound` on an unknown key, empty
  response — so there is one delete semantics across the whole surface.
- **Predictable and cheap.** One indexed multi-delete on `document_id`; no chain to walk, no `is_latest` flip.

## Consequences

- Delete is the one **irreversible** call in an otherwise recoverable store. The UI puts it behind a confirmation step
  (a `window.confirm` on the workflow list, matching the scan-directory and agent delete affordances); the server does
  no extra guarding beyond the auth policy (`tasks` group).
- `versioned.Store` now has a history-destroying method. Any future consumer that needs true immutability must not call
  `DeleteAllVersions`; the method name is deliberately explicit so a reviewer notices.
- No data migration: the collection and its documents are unchanged.

## Considered options

- **Soft delete via a tombstone version.** Rejected: every read path in a generic component grows a special case, for a
  capability nothing needs.
- **Delete only the latest version.** Rejected: it would resurrect version _n-1_ as the new latest, which is a surprise,
  not a delete.
- **Refuse to delete; only allow archiving via a tag.** Rejected: leaves the "stale drafts accumulate forever" problem
  (#110) unsolved and adds a client-side filtering convention to every workflow list.
