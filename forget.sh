#!/bin/sh
# forget.sh — Automatic memory decay sidecar.
#
# Runs in a loop: lists all collections, runs forget on each, sleeps.
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
    # List all collections — extract names from JSON array
    COLLECTIONS=$("${CLAWBRAIN}" --host "${HOST}" collections 2>/dev/null)
    COUNT=$(echo "${COLLECTIONS}" | grep -o '"count":[0-9]*' | grep -o '[0-9]*')

    if [ "${COUNT}" = "0" ] || [ -z "${COUNT}" ]; then
        log "No collections found, sleeping."
    else
        # Extract the array value, then pull out quoted strings
        NAMES=$(echo "${COLLECTIONS}" | sed 's/.*"collections":\[//;s/\].*//' | grep -o '"[^"]*"' | tr -d '"')
        for NAME in ${NAMES}; do
            RESULT=$("${CLAWBRAIN}" --host "${HOST}" forget --collection "${NAME}" --ttl "${TTL}" 2>/dev/null) || true
            DELETED=$(echo "${RESULT}" | grep -o '"deleted":[0-9]*' | grep -o '[0-9]*')
            if [ "${DELETED}" != "0" ] && [ -n "${DELETED}" ]; then
                log "Forgot ${DELETED} memories from ${NAME}"
            fi
        done
    fi

    sleep "${INTERVAL}"
done
