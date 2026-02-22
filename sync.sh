#!/bin/sh
# sync.sh — Automatic markdown memory ingestion sidecar.
#
# Runs in a loop: syncs markdown files into ClawBrain, sleeps.
# New files are chunked, embedded, and stored as memories.
# Already-processed files are tracked in Redis and skipped.
#
# Environment variables:
#   CLAWBRAIN_HOST          Qdrant host (default: qdrant)
#   CLAWBRAIN_PORT          Qdrant gRPC port (default: 6334)
#   CLAWBRAIN_OLLAMA_URL    Ollama URL (default: http://ollama:11434)
#   CLAWBRAIN_REDIS_HOST    Redis host (default: redis)
#   CLAWBRAIN_REDIS_PORT    Redis port (default: 6379)
#   CLAWBRAIN_WORKSPACE     Base path for file discovery (default: /workspace)
#   CLAWBRAIN_SYNC_INTERVAL Sleep between cycles in seconds (default: 3600 = 1 hour)

set -e

CLAWBRAIN="/usr/local/bin/clawbrain"
HOST="${CLAWBRAIN_HOST:-qdrant}"
PORT="${CLAWBRAIN_PORT:-6334}"
OLLAMA_URL="${CLAWBRAIN_OLLAMA_URL:-http://ollama:11434}"
REDIS_HOST="${CLAWBRAIN_REDIS_HOST:-redis}"
REDIS_PORT="${CLAWBRAIN_REDIS_PORT:-6379}"
WORKSPACE="${CLAWBRAIN_WORKSPACE:-/workspace}"
INTERVAL="${CLAWBRAIN_SYNC_INTERVAL:-3600}"

log() {
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"
}

# Wait for Qdrant to be reachable.
log "Waiting for Qdrant at ${HOST}:${PORT}..."
while ! nc -z "${HOST}" "${PORT}" 2>/dev/null; do
    sleep 2
done
log "Qdrant is up."

# Wait for Ollama to be reachable (sync needs embeddings).
OLLAMA_HOST_PORT=$(echo "${OLLAMA_URL}" | sed 's|http://||' | sed 's|https://||')
log "Waiting for Ollama at ${OLLAMA_HOST_PORT}..."
while ! nc -z $(echo "${OLLAMA_HOST_PORT}" | tr ':' ' ') 2>/dev/null; do
    sleep 2
done
log "Ollama is up."

# Wait for Redis to be reachable.
log "Waiting for Redis at ${REDIS_HOST}:${REDIS_PORT}..."
while ! nc -z "${REDIS_HOST}" "${REDIS_PORT}" 2>/dev/null; do
    sleep 2
done
log "Redis is up."

log "Starting sync loop: workspace=${WORKSPACE}, interval=${INTERVAL}s"

while true; do
    RESULT=$("${CLAWBRAIN}" \
        --host "${HOST}" \
        --port "${PORT}" \
        --ollama-url "${OLLAMA_URL}" \
        --redis-host "${REDIS_HOST}" \
        --redis-port "${REDIS_PORT}" \
        sync --base "${WORKSPACE}" 2>/dev/null) || true

    ADDED=$(echo "${RESULT}" | grep -o '"added":[0-9]*' | grep -o '[0-9]*')
    if [ "${ADDED}" != "0" ] && [ -n "${ADDED}" ]; then
        log "Synced ${ADDED} new chunks"
    fi

    sleep "${INTERVAL}"
done
