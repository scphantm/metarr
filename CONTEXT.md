# Metarr

Metarr manages metadata for a media library. A server owns the database and the
configuration; agents run on the machines that hold the files, scanning them and
reading or writing sidecar metadata. Work is described as workflows and executed
by nodes.

## Language

### Configuration

**Application config**:
The single document holding every runtime-adjustable setting for the system. One
document exists; there is no second copy and no per-environment variant.
_Avoid_: settings, app settings, system config

**Config store**:
The one module through which every change to the application config passes. It
reads the current document, applies the change, and announces it. Its own read
capability exists only to serve startup bootstrap, before live config exists —
general server code never calls it.
_Avoid_: config service, config writer, config repo

**Live config**:
The in-process copy of the application config every server-side read (outside
bootstrap) uses. Kept current by the config store's mutation listener, which
writes it only after a mutation is durably persisted — never read directly
from storage.
_Avoid_: cached config, current config, config snapshot

**Config mutation**:
A single named change to the application config, applied to a document the config
store has just read. A mutation names what it changes; it never supplies a whole
document.
_Avoid_: config update, config write

**Bootstrap**:
The one-time seeding of the application config at server startup, before the
config store's mutation path is live to fire or consume events. Applied
synchronously, straight to storage — it is not a mutation and never fires
`system_config_update`, because nothing is running yet to consume it.
_Avoid_: config mutation, seed data, startup config

**Projection**:
The redacted per-agent view of the application config. An agent reads its own
projection and never the document itself, which carries every credential.
_Avoid_: agent config, agent view

### Identity

Two idioms exist, and which one a thing uses is a decision, not an accident.

**Slug**:
A short human-chosen name that identifies a thing the operator named — an agent,
a scanner, a Sonarr instance. Unique, stable, and typed by a person, so it can
appear in paths and channel names.
_Avoid_: name, key, handle

**Minted id**:
An opaque identifier the server generates once and never reuses, for a thing with
no natural name — a sidecar type, an API key entry. It survives renaming, so an
entry stays addressable when every visible field changes.
_Avoid_: uuid, guid

**API key entry**:
One issued key, held in an access-level group. Its name is optional and not
unique, so it is identified by a minted id.
_Avoid_: token, credential
