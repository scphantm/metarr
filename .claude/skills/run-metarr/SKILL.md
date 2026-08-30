---
name: run-metarr
description: Build, run, and smoke-test Metarr (server, agent, UI). Use when asked to start Metarr, run the full stack, take a screenshot, or verify the app is working.
---

This skill builds and runs the complete Metarr stack — the Go HTTP API server, the Go agent process, and the Vite/React UI — and verifies end-to-end via smoke tests (heartbeat check, optional login, dashboard screenshot).

The stack has three components that start together:
- **metarr-server** (`./bin/metarr-server`) on port 8080, needs MongoDB + Redis, reads `config.local.yaml`
- **metarr-agent** (`./bin/metarr-agent`) with slug `local`, connects to Redis only
- **Metarr UI** (`npm run dev` in `ui/`) on port 5173, proxies `/api` to the server

All paths below are relative to the repo root.

## Prerequisites

- **Docker** (Rancher Desktop or native). Images (`mongo:latest`, `redis:7-alpine`) already pulled locally. Ports 8080, 5173, 27017, 6379 must be free.
- **Go 1.26** (or `go version` to check).
- **Node.js + npm** (for the UI dev server). No `.nvmrc` in `ui/`, but npm is available.
- **curl** + **jq** (for API smoke tests).
- **Playwright** (one-time install): After first `npm install` in this session, run `npx playwright install chromium` (downloads a browser binary, ~300MB). This is run as part of the setup if needed.

## Setup

From repo root:

```bash
docker compose up -d          # Bring up mongodb:27017, redis:6379 (pass "metarr"), mongo-express, openobserve, fluent-bit, redis-insight
docker compose ps --status=running  # Verify mongodb and redis are healthy
yarn workspace @metarr/metarr-ui install  # Install UI dependencies (one-time) — matches `make ui-install`; the repo's real lockfile is root yarn.lock, not ui/package-lock.json
npx playwright install chromium  # Install browser (one-time, if not already done)
```

## Build

All three components are built fresh each run:

```bash
go build -o bin/metarr-server ./cmd/metarr-server
go build -o bin/metarr-agent ./cmd/metarr-agent
yarn workspace @metarr/metarr-ui run build   # Optional; dev server builds on demand. Matches `make ui-build` (also syncs ui/package.json's version from VERSION — plain `npm run build` skips that step)
```

## Run (agent path)

The agent path launches and verifies the full stack via three shell scripts in this directory. This is how a future agent will drive Metarr.

**Order:** docker-compose infra → server → agent → UI. Cleanup: reverse order. Logs → `/tmp/metarr-{server,agent,ui}.log`, screenshots → `/tmp/metarr-ui-screenshots/`.

### Step 1: Infra

```bash
docker compose up -d
# Wait a moment for mongo/redis to bind
sleep 2
docker compose ps --status=running  # Confirm healthy
```

Ports in use (verify not bound before starting):

| Service | Port | Protocol | Notes |
|---------|------|----------|-------|
| metarr-server (API) | 8080 | HTTP | Needs MongoDB + Redis (see below) |
| Vite dev (UI) | 5173 | HTTP | Proxies `/api` to localhost:8080 |
| MongoDB | 27017 | TCP | Via docker-compose |
| Redis | 6379 | TCP | Via docker-compose, password "metarr" |
| Mongo Express (admin UI) | 6969 | HTTP | Optional |
| OpenObserve | 5080 | HTTP | Optional |
| Redis Insight | 5540 | HTTP | Optional |

### Step 2: Server

```bash
# From repo root
./.claude/skills/run-metarr/smoke-server.sh
```

This script:
1. Builds `bin/metarr-server` (Go 1.26 required)
2. Sets `METARR_CONFIG_FILE=config.local.yaml` (critical: `.env` sets this but Go code doesn't load it; without this, the server tries Docker-internal hostnames `mongodb`/`redis` and fails)
3. Launches in background, redirects to `/tmp/metarr-server.log`
4. Polls `GET /api/heartbeat` (unauthenticated endpoint that round-trips Redis) until 200 OK
5. Attempts to capture admin credentials from the log (printed once on first boot; may not be present if database already bootstrapped)
6. If credentials captured: logs in via `POST /api/auth/login`, calls `GET /api/config` (authenticated) to verify the auth flow works
7. If credentials not captured: heartbeat check alone is enough verification (expected if using existing dev Mongo)

Logs: `/tmp/metarr-server.log`. Server runs on port 8080.

### Step 3: Agent

```bash
./.claude/skills/run-metarr/smoke-agent.sh
```

This script:
1. Builds `bin/metarr-agent`
2. Launches with `METARR_AGENT_SLUG=local METARR_REDIS_HOST=localhost` (connects to Redis only, no Mongo)
3. Polls `/tmp/metarr-agent.log` for the JSON startup message `"message":"agent starting"` (retry loop, 30s timeout)
4. Agent has no HTTP interface, so log grep is the readiness probe

Logs: `/tmp/metarr-agent.log`. Agent emits JSON-formatted log lines.

### Step 4: UI + End-to-end smoke

```bash
cd ui && npm run dev > /tmp/metarr-ui.log 2>&1 &
UI_PID=$!

# Wait for Vite dev server to come up (serves on :5173)
timeout 30 bash -c 'until curl -sf http://localhost:5173 >/dev/null; do sleep 1; done'

# If credentials were captured in Step 2, login and verify dashboard
# Otherwise just verify the login screen renders
node ./.claude/skills/run-metarr/smoke-ui.mjs [admin_username] [admin_password]
```

The UI script (`smoke-ui.mjs`):
- Navigates to `http://localhost:5173`
- If credentials provided: fills the login form (no `id`/`name`/`data-testid` on inputs — selected by position and type), clicks "Sign in", waits for the "System" heading (the dashboard), proves end-to-end working
- If no credentials: just verifies the login form renders
- Takes a screenshot → `/tmp/metarr-ui-screenshots/latest.png`

Logs: `/tmp/metarr-ui.log`.

### Step 5: Cleanup

Each step above (`smoke-server.sh`, `smoke-agent.sh`, `npm run dev`) runs as its own backgrounded process from its own command invocation, so a `$SERVER_PID`/`$AGENT_PID`/`$UI_PID` captured in one command's shell is not visible to a later command — the harness does not persist shell state (env vars, backgrounded job table) between separate tool calls, only the working directory. Kill by matching the process, not by a variable from an earlier step:

```bash
# Kill processes (reverse order of launch)
pkill -f 'npm run dev' || true
pkill -f 'bin/metarr-agent' || true
pkill -f 'bin/metarr-server' || true

# Shut down docker-compose (does NOT wipe volumes — preserves dev data)
docker compose down
```

This matches the project's CLAUDE.md rule: **always kill spawned processes before ending a turn.** (`make down` runs the same three `pkill` patterns plus `docker compose down` in one shot, if you launched via `make run` instead of the smoke scripts.)

---

## Run (human path)

If you're not an agent and just want to run Metarr locally (e.g., for development):

```bash
# Terminal 1: infra
docker compose up

# Terminal 2: server
export METARR_CONFIG_FILE=config.local.yaml
go run ./cmd/metarr-server

# Terminal 3: agent
export METARR_AGENT_SLUG=nas-01 METARR_REDIS_HOST=localhost
go run ./cmd/metarr-agent

# Terminal 4: UI
cd ui && npm run dev    # → opens on http://localhost:5173
```

To stop: Ctrl-C in each terminal.

---

## Gotchas

- **`.env` is not auto-loaded by Go.** The repo's `.env` file sets `METARR_CONFIG_FILE=config.local.yaml`, but nothing in the Go code or Makefile reads it. The smoke scripts export it explicitly before launching the server. Without this, the server silently tries to resolve Docker-internal hostnames (`mongodb`, `redis`) and fails.

- **Backgrounding `go run` doesn't give you the real process PID.** `go run ./cmd/metarr-server &` spawns an npm-like wrapper that forks the actual binary; capturing `$!` and later `kill $!` leaves the real server running. The smoke scripts use `go build -o bin/metarr-{server,agent}` instead so the captured PID is the real process.

- **Admin credentials print only once, on the first boot ever.** If the MongoDB volume already exists (e.g., from a prior `docker compose up` in this project), the bootstrap check finds the existing admin user and skips the one-time credential print. The credentials are bcrypt-hashed in the database and not recoverable. The smoke scripts treat this as a normal outcome and skip the authenticated smoke test when no credentials are found.

- **The UI login form has no `id`/`name`/`data-testid` attributes** — inputs must be selected by position/type (first plain text input = username, `input[type="password"]` = password) or by button text. The Playwright script handles this.

---

## Troubleshooting

### Server fails to start with "dial unix /var/run/docker.sock: permission denied"

You don't have Docker access. Verify `docker ps` works (check user's docker group membership or `sudo`).

### Server fails to start with "failed to connect to MongoDB"

The docker-compose services haven't come up yet or ports aren't bound. Run `docker compose ps --status=running` to check. If containers exist but aren't healthy, try `docker compose down && docker compose up -d`.

### Server fails to start with "i/o timeout" connecting to `mongodb`/`redis`

`METARR_CONFIG_FILE` is not set. The server is trying to resolve Docker-internal hostnames (`mongodb`, `redis`) instead of `localhost`. Verify `export METARR_CONFIG_FILE=config.local.yaml` before launching.

### Agent fails to start with "cannot start: another agent is already running with this slug"

Another agent process with slug `local` is running. Kill it: `pkill -f 'bin/metarr-agent.*local'` or check running containers (`docker ps -a | grep agent`).

### UI doesn't load or login fails

- Verify the server is up: `curl http://localhost:8080/api/heartbeat` should return `{"time":"...", "correlation_id":"..."}` (200 OK).
- Verify the server logs don't show auth errors: `tail /tmp/metarr-server.log | grep -i auth`.
- If credentials weren't captured and you don't know the admin password: the database already existed, and credentials are in Mongo (or use `docker compose down -v` to wipe it, but this destroys dev data).

### Playwright install fails ("Could not find chromium binary")

Run `npx playwright install chromium` manually from the repo root. This downloads the browser (~300MB) and may take a minute.

### Screenshot is blank or shows an error page

Check the browser console for JS errors: the Playwright script doesn't currently capture console output. Try running `npm run dev` in `ui/` manually and opening `http://localhost:5173` in your browser to debug. Also check the server's `/api` responses for errors (see Server fails above).