#!/bin/sh
# forget.sh — Automatic memory decay sidecar.
#
# Runs in a loop: runs forget on the single memories collection, sleeps.
# Memories that haven't been accessed within the TTL fade away naturally.
#
# Environment variables:
#   CLAWBRAIN_HOST     Qdrant host (default: qdrant)
#   CLAWBRAIN_PORT     Qdrant gRPC port (default: 6334)
#   CLAWBRAIN_TTL      Forget TTL (default: 720h = 30 days)
#   CLAWBRAIN_INTERVAL Sleep between cycles in seconds (default: 3600 = 1 hour)

set -e

CLAWBRAIN="/usr/local/bin/clawbrain"
HOST="${CLAWBRAIN_HOST:-qdrant}"
PORT="${CLAWBRAIN_PORT:-6334}"
TTL="${CLAWBRAIN_TTL:-720h}"
INTERVAL="${CLAWBRAIN_INTERVAL:-3600}"

log() {
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"
}

# Wait for Qdrant to be reachable.
# Use a direct TCP probe rather than 'clawbrain check' — check also verifies
# Ollama, which forget never calls (no embeddings needed for pruning). If
# Ollama is temporarily unavailable, the forget sidecar would otherwise block
# indefinitely even though Qdrant is healthy and forget can run normally.
log "Waiting for Qdrant at ${HOST}:${PORT}..."
while ! nc -z "${HOST}" "${PORT}" 2>/dev/null; do
    sleep 2
done
log "Qdrant is up."

log "Starting forget loop: ttl=${TTL}, interval=${INTERVAL}s"

while true; do
    RESULT=$("${CLAWBRAIN}" --host "${HOST}" --port "${PORT}" forget --ttl "${TTL}" 2>/dev/null) || true
    DELETED=$(echo "${RESULT}" | grep -o '"deleted":[0-9]*' | grep -o '[0-9]*')
    if [ "${DELETED}" != "0" ] && [ -n "${DELETED}" ]; then
        log "Forgot ${DELETED} memories"
    fi

    sleep "${INTERVAL}"
done
