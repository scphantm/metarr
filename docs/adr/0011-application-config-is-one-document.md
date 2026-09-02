---
status: accepted
---

# The application config is one document, not a document per resource

ADR-0010 reshapes the config API into per-resource CRUD: `CreateAgent`, `UpdateSonarrInstance`, `DeleteScanDirectory`.
Storage could follow the API into a document per resource. It does not — the application config stays one `app_config`
singleton document, the whole `Config` message serialised as protojson, and per-resource writes are read-modify-write on
that document under the config store's lock (ADR-0002).

## Why

- **Nothing reads config from MongoDB at runtime.** The server reads the in-process live-config singleton; agents read
  their redacted Redis projection. Mongo is touched only by startup bootstrap (one load) and the write path. Splitting
  to enable partial reads would optimise a path that is not taken.
- **The store lock already serialises every config write** (ADR-0002), so a write that needs to read sibling sections —
  the ones cross-entry validation spans — gets a consistent view without a Mongo multi-document transaction. Splitting
  would trade a single-document atomic write for either a transaction or an accepted inconsistency window, for no gain.
- **Cross-entry validation needs the whole config in hand regardless.** The sidecar-registry compile, the Sonarr
  cross-type slug-uniqueness check, and agent mapping validation all run against the full configuration at write time
  whatever the storage shape.
- **The data is operator-bounded.** Agents, Sonarr instances, scan directories, sidecar types, and API keys number in
  the dozens — low hundreds at the extreme — kilobytes of document. MongoDB's 16 MB limit is not in sight and there is
  no read or write cost to reduce. The large datasets this system will hold (scan records, metadata) belong to other
  services, not this one.
- **One document is one repo, one read path, one write path, one shape** — the `Config` message. ADR-0005's "the message
  is the stored document" stays literally true.

## Considered options

- **A document per resource type** (a Mongo collection per config collection — `agents`, `sonarr_instances`, …).
  Rejected now: the aggregate `GetConfig` read and the agent projection become an assembly of half a dozen reads;
  `Normalize`, `Bootstrap`, and `BuildProjection` all learn to assemble; the repo layer multiplies — and none of it buys
  measurable efficiency at this size. The store lock already provides the consistency a transaction would, so it is not
  an atomicity win either.

## Consequences

- `List` pagination, filtering, and ordering (ADR-0010) are applied to the in-memory slices lifted from the singleton,
  not pushed down to a Mongo query. Acceptable at operator scale.
- **Graduation path.** If one collection ever becomes genuinely large (thousands of rows) or write-hot, that collection
  alone moves to its own Mongo collection with per-document storage and a cursor-backed `List`; the scalar sections and
  every other collection stay in the singleton. This is a per-collection change made on evidence, not a wholesale
  restructuring and not something to pre-build.
