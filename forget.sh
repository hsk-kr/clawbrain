#!/bin/sh
# forget.sh — Automatic memory decay sidecar.
#
# Runs in a loop: runs forget on the single memories collection, sleeps.
# Memories that haven't been accessed within the TTL fade away naturally.
#
# Environment variables:
#   CLAWBRAIN_HOST    Qdrant host (default: qdrant)
#   CLAWBRAIN_TTL     Forget TTL (default: 720h = 30 days)
#   CLAWBRAIN_INTERVAL Sleep between cycles in seconds (default: 3600 = 1 hour)

set -e

CLAWBRAIN="/usr/local/bin/clawbrain"
HOST="${CLAWBRAIN_HOST:-qdrant}"
TTL="${CLAWBRAIN_TTL:-720h}"
INTERVAL="${CLAWBRAIN_INTERVAL:-3600}"

log() {
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"
}

# Wait for Qdrant to be reachable
log "Waiting for Qdrant at ${HOST}:6334..."
while ! "${CLAWBRAIN}" --host "${HOST}" check >/dev/null 2>&1; do
    sleep 2
done
log "Qdrant is up."

log "Starting forget loop: ttl=${TTL}, interval=${INTERVAL}s"

while true; do
    RESULT=$("${CLAWBRAIN}" --host "${HOST}" forget --ttl "${TTL}" 2>/dev/null) || true
    DELETED=$(echo "${RESULT}" | grep -o '"deleted":[0-9]*' | grep -o '[0-9]*')
    if [ "${DELETED}" != "0" ] && [ -n "${DELETED}" ]; then
        log "Forgot ${DELETED} memories"
    fi

    sleep "${INTERVAL}"
done
