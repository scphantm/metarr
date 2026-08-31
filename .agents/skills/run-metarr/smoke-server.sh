#!/usr/bin/env bash
set -euo pipefail

# Smoke-test the metarr-server: build, launch, poll for readiness, verify, capture credentials, stop.
# Called by SKILL.md as part of the full-stack test sequence.
# Inputs: none (reads from repo root, logs to /tmp)
# Outputs: $SERVER_PID (backgrounded process), /tmp/metarr-server.log, and optionally captured credentials
# Exit code: 0 on success, nonzero on failure

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$REPO_ROOT"

LOG_FILE="/tmp/metarr-server.log"

echo "=== Building metarr-server ==="
go build -o bin/metarr-server ./cmd/metarr-server

echo "=== Launching metarr-server ==="
METARR_CONFIG_FILE=config.local.yaml ./bin/metarr-server > "$LOG_FILE" 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

echo "=== Waiting for server readiness ==="
for i in {1..30}; do
  if curl -sf http://localhost:8080/api/heartbeat > /dev/null 2>&1; then
    echo "Server is ready (heartbeat responded)"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Server did not become ready after 30 seconds"
    kill "$SERVER_PID" 2>/dev/null || true
    return 1
  fi
  sleep 1
done

echo "=== Smoke test: unauthenticated heartbeat ==="
HEARTBEAT=$(curl -sf http://localhost:8080/api/heartbeat)
echo "Heartbeat response: $HEARTBEAT"
echo ""

echo "=== Attempting to capture admin credentials ==="
ADMIN_USERNAME=$(grep -oP '(?<=  username: ).*' "$LOG_FILE" || echo "")
ADMIN_PASSWORD=$(grep -oP '(?<=  password: ).*' "$LOG_FILE" || echo "")

if [ -z "$ADMIN_PASSWORD" ]; then
  echo "No credentials printed (expected if database already bootstrapped)"
  echo "Skipping authenticated smoke test"
else
  echo "Credentials captured: username=$ADMIN_USERNAME"

  echo "=== Smoke test: login and authenticated call ==="
  LOGIN_RESPONSE=$(curl -sf -X POST http://localhost:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$ADMIN_USERNAME\",\"password\":\"$ADMIN_PASSWORD\"}")
  echo "Login response: $LOGIN_RESPONSE"

  # Extract API key from response (simple string parsing)
  API_KEY=$(echo "$LOGIN_RESPONSE" | grep -oP '(?<="api_key":")[^"]*' || echo "")
  if [ -z "$API_KEY" ]; then
    echo "ERROR: Failed to extract API key from login response"
    kill "$SERVER_PID" 2>/dev/null || true
    return 1
  fi
  echo "API Key obtained: ${API_KEY:0:8}..."

  echo "=== Smoke test: authenticated /api/config call ==="
  CONFIG=$(curl -sf -H "X-Api-Key: $API_KEY" http://localhost:8080/api/config)
  echo "Config response (first 200 chars): ${CONFIG:0:200}..."
  echo ""
fi

# Export variables for caller to use in subsequent steps
export SERVER_PID
export ADMIN_USERNAME
export ADMIN_PASSWORD

echo "=== Server smoke test complete ==="
echo "To stop: kill $SERVER_PID"