---
status: accepted
---

# No credential travels in a bus payload

Once a participant (ADR-0008) can attach to Redis with nothing but the connection credential, it can `XREAD` any stream and `SUBSCRIBE` any channel; Redis does not scope a subscription to a role. ADR-0006 already redacts the per-agent projection so an agent never sees the admin hash or another integration's API key, but that redaction is a property of one payload, not of the bus. This ADR makes it a property of the bus.

## Decision

No message published to the event bus — on a durable stream or a Pub/Sub channel, envelope or inner payload — may contain a credential: no admin password hash or salt, no API key, no Sonarr or other third-party token, no session token, no Redis or Mongo connection string. A value a participant must not read does not go on the bus. Where the bus needs to refer to a credentialed thing, it carries an identifier — an instance slug, a key's minted id — and the holder resolves the secret out of band through a redacted read (the projection, a scoped config RPC).

Participants share the one Redis connection credential. Per-participant Redis ACL users, scoped to the key patterns and channels a role needs, are a later hardening and are not required by this decision.

## Why

The alternative is to treat Redis ACLs as the control and let payloads carry whatever they carry. That turns every new payload into a security review — "who can subscribe to this stream, and is this field safe for them" — makes the ACL rules load-bearing for confidentiality rather than for tidiness, and fails open: a missing ACL rule exposes data, a missing "don't put the token here" leaves a token on a 48-hour stream. Keeping secrets off the bus entirely fails closed — the worst a broad subscription yields is paths and metadata.

## Consequences

- A payload that needs to act on a credentialed external system carries the slug or minted id, and the server or the participant's operator supplies the actual secret out of band. This is already how the agent projection works; the ADR generalises it.
- Streams retain ~48 hours of history (ADR-0006). A secret published in error is not gone when the code is fixed — it sits on the stream until it ages out or the stream is purged. The invariant is worth an explicit check in review for every new bus payload.
- `AdminUser` on the config RPCs still carries always-empty `password_salt` / `password_hash` fields (ADR-0005); that message is not a bus payload and is unaffected.

## Out of scope

Per-participant Redis ACLs. Encrypting payloads at rest in Redis. Authenticating which process published an event — the `source` field is asserted, not verified — which belongs with participant identity, deferred by ADR-0008.
