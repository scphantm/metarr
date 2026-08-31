#!/usr/bin/env bash
set -euo pipefail

# Smoke-test the metarr-agent: build, launch, poll for readiness (log grep), stop.
# Called by SKILL.md as part of the full-stack test sequence.
# Inputs: none (reads from repo root, logs to /tmp)
# Outputs: $AGENT_PID (backgrounded process), /tmp/metarr-agent.log
# Exit code: 0 on success, nonzero on failure

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$REPO_ROOT"

LOG_FILE="/tmp/metarr-agent.log"

echo "=== Building metarr-agent ==="
go build -o bin/metarr-agent ./cmd/metarr-agent

echo "=== Launching metarr-agent (slug: local) ==="
METARR_AGENT_SLUG=local METARR_REDIS_HOST=localhost ./bin/metarr-agent > "$LOG_FILE" 2>&1 &
AGENT_PID=$!
echo "Agent PID: $AGENT_PID"

echo "=== Waiting for agent readiness ==="
timeout 30 bash -c "until grep -q '\"message\":\"agent starting\"' '$LOG_FILE'; do sleep 0.5; done" || {
  echo "ERROR: Agent did not start within 30 seconds"
  kill "$AGENT_PID" 2>/dev/null || true
  return 1
}

echo "Agent is ready (startup message logged)"

# Show the startup log entry
echo "=== Agent startup confirmation ==="
grep '"message":"agent starting"' "$LOG_FILE" | head -1

export AGENT_PID

echo "=== Agent smoke test complete ==="
echo "To stop: kill $AGENT_PID"