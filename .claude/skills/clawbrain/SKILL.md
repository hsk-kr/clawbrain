---
name: clawbrain
description: Guide for AI agents to use ClawBrain, a vector memory engine providing persistent semantic storage and similarity-based retrieval. Use this when working with the ClawBrain codebase or integrating ClawBrain as agent memory infrastructure.
---

# ClawBrain

Vector memory engine for AI agents.

## What This Is

ClawBrain is a **memory infrastructure layer** -- persistent semantic storage and similarity-based retrieval for AI agents. Think Redis for key-value, but for semantic vectors.

It is **recall**, not cognition. Agents stay in control of reasoning.

### ClawBrain provides

- Persistent semantic memory (vectors + metadata)
- Similarity-based retrieval with deterministic controls (thresholds, limits)
- Structured metadata (payload) storage
- Collection-based namespacing
- Automatic memory decay (forgetting)
- Access tracking (last-accessed timestamps)

### ClawBrain does NOT

- Think, reason, or make decisions
- Generate embeddings
- Call LLMs
- Influence what an agent thinks next
- Orchestrate multi-hop retrieval (agents do this)

## Design Philosophy

These principles are non-negotiable. Every contribution must respect them:

- **Memory as infrastructure** -- a system component, not a framework
- **Agent-first** -- designed for AI consumption, not human UI
- **Clean separation** -- memory and reasoning are distinct layers, never blended
- **Deterministic retrieval** -- predictable behavior with explicit controls, no magic
- **Minimal and opinionated** -- a Unix tool that does one thing well
- **Biological inspiration** -- forgetting and dreaming are features, not bugs
- **JSON in, JSON out** -- agents parse structured output, always

## Architecture

### Stack

| Component | Version | Purpose |
|---|---|---|
| Docker | -- | Runs Qdrant and builds binaries (only hard dependency) |
| Go | 1.25 | Application language (runs inside Docker for builds) |
| Qdrant | v1.17.0 | Vector database (Docker container) |
| go-client | v1.17.1 | Qdrant Go gRPC client |

### Project Structure

```
build/                 # Cross-compiled binaries (committed, built by pre-commit hook)
build.sh               # Cross-compile script (uses Docker, no local Go needed)
cmd/
  clawbrain/
    main.go            # CLI entrypoint -- command dispatch, flag parsing, JSON output
    main_test.go       # End-to-end CLI integration tests (builds binary, runs as subprocess)
  check/
    main.go            # Standalone connectivity check (early prototype, not used by CLI)
internal/
  store/
    store.go           # Core Qdrant store abstraction -- all database operations
    store_test.go      # Store unit + integration tests
docker-compose.yml     # Qdrant container (ports 6333 REST, 6334 gRPC)
```

### Data Flow

```
Agent
  |
  |-- clawbrain add --collection X --vector '[...]' --payload '{...}'
  |     -> main.go:runAdd -> store.Add -> qdrant.Upsert
  |        (auto-creates collection, injects timestamps, generates UUID)
  |
  |-- clawbrain retrieve --collection X --vector '[...]' --min-score 0.7
  |     -> main.go:runRetrieve -> store.Retrieve -> qdrant.Query
  |        (filters by score, updates last_accessed on hits)
  |
  |-- clawbrain forget --collection X --ttl 720h
  |     -> main.go:runForget -> store.Forget -> qdrant.Scroll + qdrant.Delete
  |        (finds stale points, batch deletes them)
  |
  |-- clawbrain check
        -> main.go:runCheck -> store.Check -> create/upsert/query/delete
           (end-to-end connectivity verification)
```

All output is JSON to stdout. All errors are JSON to stdout with exit code 1.

### Key Code Locations

- **CLI dispatch and flag parsing**: `cmd/clawbrain/main.go`
- **Store abstraction (all Qdrant operations)**: `internal/store/store.go`
- **Collection auto-creation**: `internal/store/store.go` -- `ensureCollection` (cosine distance, vector size inferred from input)
- **Timestamp injection**: `internal/store/store.go` -- `Add` method injects `created_at` and `last_accessed` (RFC3339Nano)
- **Last-accessed update on retrieval**: `internal/store/store.go` -- `updateLastAccessed` (fire-and-log, never fails retrieval)
- **Paginated scroll for forget**: `internal/store/store.go` -- `scrollPointIDs` (batch size 100)
- **Protobuf value conversion**: `internal/store/store.go` -- `valueMapToGoMap` / `valueToGo`

### Configuration

There are no config files or environment variables. Everything is hardcoded constants or CLI flags:

| Setting | Value | Location |
|---|---|---|
| Qdrant host | `localhost` | `cmd/clawbrain/main.go` |
| Qdrant gRPC port | `6334` | `cmd/clawbrain/main.go` |
| Context timeout | `30s` | `cmd/clawbrain/main.go` |
| Default TTL | `720h` (30 days) | CLI flag default |
| Default min-score | `0.0` | CLI flag default |
| Default limit | `1` | CLI flag default |
| Distance metric | Cosine | `internal/store/store.go` |

## CLI Reference

All commands output JSON. All commands require Qdrant running on `localhost:6334`.

### `clawbrain add`

Store a memory.

```bash
clawbrain add --collection <name> --vector '<json array>' --payload '<json object>' [--id <uuid>]
```

| Flag | Required | Description |
|---|---|---|
| `--collection` | yes | Target collection name |
| `--vector` | yes | Embedding vector as JSON array |
| `--payload` | yes | Metadata as JSON object |
| `--id` | no | UUID for the point (auto-generated if omitted) |

Automatically adds `created_at` and `last_accessed` timestamps to the payload. Auto-creates the collection if it doesn't exist (vector dimension inferred from input).

### `clawbrain retrieve`

Retrieve similar memories.

```bash
clawbrain retrieve --collection <name> --vector '<json array>' [--min-score 0.7] [--limit 1]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Collection to search |
| `--vector` | yes | -- | Query embedding as JSON array |
| `--min-score` | no | `0.0` | Minimum similarity score threshold |
| `--limit` | no | `1` | Maximum number of results |

Updates `last_accessed` on all returned memories (keeps them alive for decay).

### `clawbrain forget`

Prune stale memories.

```bash
clawbrain forget --collection <name> [--ttl 720h]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Collection to prune |
| `--ttl` | no | `720h` | Duration -- memories not accessed within this window are deleted |

### `clawbrain check`

Connectivity check. No flags.

```bash
clawbrain check
```

Runs an end-to-end test: creates a temp collection, upserts a vector, queries it, deletes the collection.

## Conceptual Model

### Collections

Namespaces for organizing vectors. Auto-created on first `add`. Use them to separate memory domains (e.g., per-agent, per-task, per-topic).

### Vectors

Embeddings generated externally by the agent. ClawBrain stores and compares them using cosine similarity. Dimensionality is locked per collection (set by the first vector added).

### Payload

Structured metadata (JSON object) attached to each vector. The agent controls what goes in here. ClawBrain automatically manages two fields:

- `created_at` -- set once on `add`
- `last_accessed` -- updated on every `retrieve` hit

### Memory Decay (Forgetting)

Memories that are never recalled fade away -- like human memory:

- Every memory stores a `last_accessed` timestamp
- `retrieve` updates `last_accessed` on every hit
- `forget` prunes memories not accessed within a configurable TTL
- Can be run manually, as a cron job, or as a scheduled service

### Agent Interaction Pattern

1. Agent generates embedding for a memory
2. Agent stores vector + metadata via `clawbrain add`
3. Later, agent generates embedding for a query
4. Agent retrieves top matches via `clawbrain retrieve`
5. Agent applies thresholds and uses results in its own reasoning

### Multi-Hop Retrieval (Agent-Side)

ClawBrain provides single-hop retrieval. The agent orchestrates multi-hop recall:

1. Send query vector to `clawbrain retrieve` -- get top match + score
2. If score < threshold -- stop, no relevant memory found
3. Combine original thought + retrieved memory into new context
4. Generate new embedding from combined context
5. Send new query to `clawbrain retrieve`
6. Repeat until max hops, score below threshold, or context limit reached

This keeps reasoning in the agent's hands. ClawBrain just recalls.

### Dreaming (Future)

A planned process that remixes existing memories to create new associations:

- Retrieve random memories from a collection
- An external agent combines/remixes them into new "dream" memories
- Store dream memories back with a `source: dream` tag
- Purpose: surface unexpected connections, consolidate related memories

The dreaming orchestration lives in the agent layer. ClawBrain provides the primitives.

## Development

### Prerequisites

- **Docker** and **Docker Compose** -- the only hard dependency
  - Qdrant runs as a Docker container
  - Builds use a Docker Go image (no local Go installation needed)
- Go 1.25+ (optional, only if you want to run/test locally without Docker)

### Running

```bash
# Start Qdrant
docker compose up -d

# Verify Qdrant
curl http://localhost:6333

# Run connectivity check
go run ./cmd/clawbrain check

# Stop Qdrant
docker compose down
```

### Building

Builds use Docker -- no local Go installation needed.

```bash
# Cross-compile all platforms (outputs to build/)
./build.sh

# Or build locally if you have Go installed
go build -o clawbrain ./cmd/clawbrain
```

A **pre-commit git hook** runs `build.sh` automatically before every commit, so `build/` always contains up-to-date binaries for all platforms. The hook stages the binaries into the commit.

Cross-compile targets:

| OS | Arch | Binary |
|---|---|---|
| Linux | amd64 | `build/clawbrain-linux-amd64` |
| Linux | arm64 | `build/clawbrain-linux-arm64` |
| macOS | amd64 (Intel) | `build/clawbrain-darwin-amd64` |
| macOS | arm64 (Apple Silicon) | `build/clawbrain-darwin-arm64` |
| Windows | amd64 | `build/clawbrain-windows-amd64.exe` |

All binaries are statically linked (`CGO_ENABLED=0`), built inside a `golang:1.25` Docker container.

### Testing

```bash
# Store tests (requires Qdrant running)
go test ./internal/store/ -v

# CLI integration tests (requires Qdrant running)
go test ./cmd/clawbrain/ -v

# All tests
go test ./... -v
```

### Ports

- `6333` -- Qdrant REST API
- `6334` -- Qdrant gRPC (used by Go client)

## Scaling Direction

Future capabilities (always storage + retrieval, never reasoning):

- Personal agent memory
- Shared team memory
- Multi-agent collaborative memory
- Partitioned memory domains
- Memory pruning policies
- Dreaming / memory consolidation (agent-orchestrated)
