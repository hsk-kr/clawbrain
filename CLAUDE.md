# ClawBrain

Vector memory engine for AI agents.

## What It Is

ClawBrain is a **memory infrastructure layer** — persistent semantic storage and similarity-based retrieval for AI agents.

It provides:
- Persistent semantic memory (vectors + metadata)
- Similarity-based retrieval with deterministic controls (thresholds, limits)
- Structured metadata (payload) storage
- Collection-based namespacing
- Automatic memory decay (forgetting)
- Access tracking (last-accessed timestamps)

## What It Is NOT

ClawBrain does **not**:
- Think, reason, or make decisions
- Generate embeddings
- Call LLMs
- Influence what an agent thinks next
- Orchestrate multi-hop retrieval (agents do this)

It is **recall**, not cognition. Agents stay in control of reasoning.

## Conceptual Model

- **Collections** — namespaces for organizing vectors
- **Vectors** — embeddings (generated externally by the agent)
- **Payload** — structured metadata attached to vectors
- **Similarity scores** — ranked retrieval results
- **Retrieval constraints** — thresholds, limits, filters
- **Memory decay** — memories not accessed within a TTL are automatically forgotten
- **Last-accessed tracking** — every retrieval updates the memory's access timestamp

### Agent Interaction Pattern

1. Agent generates embedding for a memory
2. Agent stores vector + metadata in ClawBrain via `clawbrain add`
3. Later, agent generates embedding for a query
4. Agent retrieves top matches from ClawBrain via `clawbrain retrieve`
5. Agent applies thresholds and uses results in its own reasoning

### Multi-Hop Retrieval Protocol (Agent-Side)

ClawBrain provides single-hop retrieval. The agent orchestrates multi-hop recall:

1. Agent sends query vector to `clawbrain retrieve` — gets top match + score
2. If score < `min-score` — stop, no relevant memory found
3. Agent combines original thought + retrieved memory into new context
4. Agent generates new embedding from combined context
5. Agent sends new query to `clawbrain retrieve`
6. Repeat until:
   - Max hops reached (`--hops` limit)
   - Top result score < `min-score`
   - Combined context exceeds `--context-length`

This keeps reasoning in the agent's hands. ClawBrain just recalls.

### Memory Decay (Forgetting)

Memories that are never recalled fade away — like human memory:

- Every memory stores a `last_accessed` timestamp in its payload
- `clawbrain retrieve` updates `last_accessed` on every hit
- `clawbrain forget` prunes memories not accessed within a configurable TTL
- Can be run manually, as a cron job, or as a scheduled Docker Compose service

### Dreaming (Future)

A process that remixes existing memories to create new associations:

- Retrieve random memories from a collection
- An agent (external LLM) combines/remixes them into new "dream" memories
- Store dream memories back into ClawBrain with a `source: dream` tag
- Purpose: surface unexpected connections, consolidate related memories

This requires an agent to do the remixing — ClawBrain provides the primitives (random retrieval, storage). The dreaming orchestration lives in the agent layer.

## CLI

ClawBrain is used via a command-line interface. All output is JSON for agent consumption.

### Commands

```bash
# Store a memory
clawbrain add --collection <name> --vector '<json array>' --payload '<json object>' [--id <uuid>]

# Retrieve similar memories
clawbrain retrieve --collection <name> --vector '<json array>' [--min-score 0.7] [--limit 1]

# Forget stale memories
clawbrain forget --collection <name> [--ttl 720h]

# Connectivity check
clawbrain check
```

### Command Details

#### `add`
| Flag | Required | Description |
|---|---|---|
| `--collection` | yes | Target collection name |
| `--vector` | yes | Embedding vector as JSON array |
| `--payload` | yes | Metadata as JSON object |
| `--id` | no | UUID for the point (auto-generated if omitted) |

Automatically adds `created_at` and `last_accessed` timestamps to the payload.

#### `retrieve`
| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | — | Collection to search |
| `--vector` | yes | — | Query embedding as JSON array |
| `--min-score` | no | `0.0` | Minimum similarity score threshold |
| `--limit` | no | `1` | Maximum number of results |

Updates `last_accessed` on all returned memories.

#### `forget`
| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | — | Collection to prune |
| `--ttl` | no | `720h` | Duration — memories not accessed within this window are deleted |

#### `check`
No flags. Runs an end-to-end connectivity check against Qdrant.

## Design Philosophy

- **Memory as infrastructure** — like Redis for key-value, ClawBrain is for semantic vectors
- **Agent-first** — designed for AI consumption, not human UI
- **Clean separation** — memory and reasoning are distinct layers
- **Deterministic retrieval** — predictable behavior with explicit controls
- **Minimal and opinionated** — a Unix tool, a system component, not a framework
- **Biological inspiration** — forgetting and dreaming are features, not bugs

## Architecture Principles

- Keep memory separate from reasoning — clear boundaries
- Predictable behavior over magic
- Replaceable LLM layer (ClawBrain doesn't care which model generates embeddings)
- Lightweight, infrastructure-minded design
- No flashy abstractions — just solid, dependable recall
- JSON in, JSON out — agents parse structured output

## Scaling Direction

Future capabilities (always as storage + retrieval, never reasoning):
- Personal agent memory
- Shared team memory
- Multi-agent collaborative memory
- Partitioned memory domains
- Memory pruning policies
- Dreaming / memory consolidation (agent-orchestrated)

## Development Setup

### Prerequisites

- **Go 1.25+**
- **Docker** and **Docker Compose**

### Stack

| Component | Version | Purpose |
|---|---|---|
| Go | 1.25 | Application language |
| Qdrant | v1.17.0 | Vector database (Docker) |
| go-client | v1.17.1 | Qdrant Go gRPC client |

### Running

```bash
# Start Qdrant
docker compose up -d

# Verify Qdrant is running
curl http://localhost:6333

# Run connectivity check
go run ./cmd/clawbrain check

# Stop Qdrant
docker compose down
```

### Building

```bash
# Build the CLI binary
go build -o clawbrain ./cmd/clawbrain

# Run directly
./clawbrain add --collection memories --vector '[0.1, 0.2, 0.3, 0.4]' --payload '{"text": "hello world"}'
./clawbrain retrieve --collection memories --vector '[0.1, 0.2, 0.3, 0.4]' --min-score 0.7
./clawbrain forget --collection memories --ttl 720h
./clawbrain check
```

### Ports

- `6333` — Qdrant REST API
- `6334` — Qdrant gRPC (used by Go client)
