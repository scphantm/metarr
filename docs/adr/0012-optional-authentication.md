---
status: accepted
---

# Authentication is optional, and "off" means implicitly the administrator

Metarr required an admin username and password to reach any part of the system: every gRPC-Web call carried an auth
interceptor, the UI opened on a login screen, and there was no way in without the generated password. For a single-user
homelab box on a trusted LAN, a demo, or a first look around, that login wall is friction with no benefit.

Authentication becomes a **scheme** chosen by the operator and stored on the admin record:

- **None** (the default on a fresh install): no login is required. A full administrator principal is synthesised for
  every request; the UI opens straight into the app.
- **Password**: exactly the previous behaviour — the admin username and password are required, a session token is minted
  on login, and every RPC is checked.

Setting the admin password is independent of the scheme. Bootstrap still generates and prints an initial admin password
on first boot under either scheme; **None** simply never demands it.

## Decisions

### 1. Authentication is optional and defaults to None on a fresh install

_Alternative: always-on, as before — a login wall on every install._

The out-of-the-box experience has no login wall. The rationale is the literal operator request: a trusted-network
install should be usable immediately, without hunting a generated password out of the server console. An operator who
wants the wall turns it on; the choice is theirs, not the installer's. This is greenfield — there are no deployed
installs to migrate and no existing-install detection.

### 2. Scheme None synthesises a full administrator principal, not an anonymous one

_Alternative: an anonymous or reduced principal that every downstream check special-cases._

Under **None** the auth interceptor attaches the administrator role and a synthetic API-key marker to the request
context and returns allowed — the same shape a real admin key produces. Downstream code, audit logging included, sees an
administrator and needs no `if scheme == None` branch. This is the smallest blast radius: no new principal type, no new
conditionals spreading through the services, and it matches the operator's mental model of "come straight in". An
anonymous principal would force every authorization site and every audit record to grow a second case that exists only
to be immediately widened back to full access.

### 3. The scheme is a field on the admin record, not a new config block

_Alternative: a dedicated top-level authentication section in the application config._

`authentication_scheme` is an enum on the existing `AdminUser` message and its `appconfig` counterpart, edited from the
Security page's administrator block alongside username, email, and password. All identity settings stay in one place.
The scheme is one enum with no siblings — a whole config section for a single field is overhead in the proto, the
struct, the builtin defaults, the CRUD surface, and the UI, with nothing to put beside it. The enum itself leaves room
for future schemes (an external identity provider, say) without reshaping the field. Config normalisation maps the
unspecified/zero value to **None**, so the default is guaranteed by the config layer, not only by a JSON file.

### 4. The pre-login read is a dedicated NoAuth RPC on the pre-login service

_Alternative: extend an authenticated admin read (`GetAdminUser`), or the REST `GET /api/heartbeat`._

`AuthService.GetAuthScheme` carries a `NoAuth` policy (like `Login`) and returns only the scheme enum. The UI calls it
on a cold load, before anyone has logged in, to decide whether to show the login gate. It cannot be a field on an
authenticated read — there is no credential yet under scheme **None**, and requiring one to learn that no credential is
required is circular. It is not bolted onto the heartbeat because the UI makes no REST calls and that stays true;
`AuthService` already owns every pre-login concern, so the probe belongs there. It is a custom read, not an AIP standard
method, and does not need that shape.

## Consequences

- The switch is ordinary config: it persists synchronously and takes effect on the very next request, with no restart
  (ADR-0002). The interceptor reads the scheme from in-process live config.
- A presented `X-Api-Key` while the scheme is **None** is ignored — not resolved, not honoured for a lower role. Broader
  API-key semantics are unchanged.
- The scheme is **not** added to the agent projection (`agentregistry.BuildProjection`). Agent connectivity is
  unaffected by the scheme under either value, and the projection allow-list stays minimal;
  `internal/agent/boundary_test.go` is unchanged.
- Existing Redis-backed session tokens are left untouched on any scheme change. Flipping **Password** → **None** →
  **Password** is not treated as a credential-compromise event; a held session stays valid until it naturally expires.
- Switching **Password** → **None** is a destructive act (it removes the login wall), so the UI confirms it with a
  dialog that names the consequence. Switching **None** → **Password** shows the admin username inline so the operator
  does not lock themselves out; no dialog.
- Lockout under **None** (anyone with network access can change the scheme) is accepted: the existing bootstrap
  password-recovery path already covers an operator who cannot log in.
