# ClawBrain

Vector memory engine for AI agents.

## What It Is

ClawBrain is a **memory infrastructure layer** — persistent semantic storage and similarity-based retrieval for AI agents.

It provides:
- Persistent semantic memory (vectors + metadata)
- Similarity-based retrieval with deterministic controls (thresholds, limits)
- Structured metadata (payload) storage
- Collection-based namespacing

## What It Is NOT

ClawBrain does **not**:
- Think, reason, or make decisions
- Generate embeddings
- Call LLMs
- Influence what an agent thinks next

It is **recall**, not cognition. Agents stay in control of reasoning.

## Conceptual Model

- **Collections** — namespaces for organizing vectors
- **Vectors** — embeddings (generated externally by the agent)
- **Payload** — structured metadata attached to vectors
- **Similarity scores** — ranked retrieval results
- **Retrieval constraints** — thresholds, limits, filters

### Agent Interaction Pattern

1. Agent generates embedding for a memory
2. Agent stores vector + metadata in ClawBrain
3. Later, agent generates embedding for a query
4. Agent retrieves top matches from ClawBrain
5. Agent applies thresholds and uses results in its own reasoning

## Design Philosophy

- **Memory as infrastructure** — like Redis for key-value, ClawBrain is for semantic vectors
- **Agent-first** — designed for AI consumption, not human UI
- **Clean separation** — memory and reasoning are distinct layers
- **Deterministic retrieval** — predictable behavior with explicit controls
- **Minimal and opinionated** — a Unix tool, a system component, not a framework

## Architecture Principles

- Keep memory separate from reasoning — clear boundaries
- Predictable behavior over magic
- Replaceable LLM layer (ClawBrain doesn't care which model generates embeddings)
- Lightweight, infrastructure-minded design
- No flashy abstractions — just solid, dependable recall

## Scaling Direction

Future capabilities (always as storage + retrieval, never reasoning):
- Personal agent memory
- Shared team memory
- Multi-agent collaborative memory
- Partitioned memory domains
- Memory pruning policies

## Development Setup

### Prerequisites

- **Go 1.26+**
- **Docker** and **Docker Compose**

### Stack

| Component | Version | Purpose |
|---|---|---|
| Go | 1.26 | Application language |
| Qdrant | v1.17.0 | Vector database (Docker) |
| go-client | v1.17.1 | Qdrant Go gRPC client |

### Running

```bash
# Start Qdrant
docker compose up -d

# Verify Qdrant is running
curl http://localhost:6333

# Run connectivity check
go run ./cmd/check

# Stop Qdrant
docker compose down
```

### Ports

- `6333` — Qdrant REST API
- `6334` — Qdrant gRPC (used by Go client)
