---
status: accepted
---

# No whole-document configuration update

`ConfigService.Update` accepted an entire configuration document from the client and replaced the stored one. Because `Get` redacts the admin password salt and hash by omitting them from the wire type, any client that round-tripped a document through `Update` wrote those fields back empty. We removed the operation: every configuration change is now a scoped operation that names exactly what it changes and is applied server-side to a freshly read document.

## Why this was urgent

The redaction and the replace combined into unrecoverable data loss. `ApiKeysSection` was the only caller, and it round-tripped the whole document on every key edit, so **adding or renaming an API key zeroed the admin password hash**. It did not self-heal: the startup bootstrap regenerates admin credentials only when `Admin.Username` is empty, and username survives the round trip, so a restart left the account locked out. Recovery originally meant editing MongoDB by hand; startup now detects an admin record with a username but no usable password and issues a new one automatically, so an instance already hit by this defect repairs itself on the next restart rather than staying locked out until this fix ships.

The defect was not a coding slip. A redacted *read* shape was accepted as a *write* shape, which is an interface flaw — no amount of care at the call site prevents it.

## Considered options

- **Merge instead of replace**: keep `Update`, but overlay only the fields the request carries onto the stored document. Rejected as a fallback rather than a fix: it leaves a general-purpose write that no caller needs, and every future field has to remember to be merge-safe.
- **Repair only the admin block**: re-read salt and hash before firing. Rejected — it fixes this instance of the pattern and leaves the pattern.

## Consequences

- There is no general-purpose configuration write. A new setting requires a scoped operation, deliberately: the operation names what it changes, so a partial view can never replace the whole.
- `APIKeyEntry` gained a minted id, because per-entry operations need to address an entry that a rename does not move. Existing stored entries are backfilled once at startup.
- Clients no longer compute the next document. This removes the client-side read-modify-write that made the bug reachable, and it is why `ApiKeysSection` no longer holds a `writeGroup` helper.
