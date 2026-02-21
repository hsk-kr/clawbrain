# ClawBrain

Vector memory engine for AI agents.

ClawBrain gives agents persistent semantic memory -- store vectors, retrieve by similarity, forget what's stale. It is infrastructure, not a framework. Recall, not cognition. Agents stay in control of reasoning.

Think Redis for key-value, but for semantic vectors.

## Why This Exists

AI agents are stateless by default. Every conversation starts from zero. ClawBrain exists so agents can remember.

But memory is not thinking. Most tools blur this line -- they inject retrieved context into prompts, orchestrate multi-hop chains, or make decisions about what's relevant. ClawBrain refuses to do any of that. It stores vectors. It returns similar vectors. That's it.

The agent decides what to remember, when to recall, and what to do with the results. ClawBrain is the filing cabinet, not the brain.

## Philosophy

- **Memory as infrastructure** -- a system component, not a framework. Like Redis, like Postgres, like S3.
- **Agent-first** -- designed for AI consumption. JSON in, JSON out. No human UI, no dashboards, no web interfaces.
- **Clean separation** -- memory and reasoning are distinct layers, never blended. ClawBrain does not call LLMs, generate embeddings, or influence what an agent thinks next.
- **Deterministic retrieval** -- predictable behavior with explicit controls. No magic ranking, no hidden reranking, no surprise behavior. You set the threshold, you get what matches.
- **Minimal and opinionated** -- a Unix tool that does one thing well. Four commands. No plugins, no extensions, no configuration files.
- **Biological inspiration** -- forgetting is a feature, not a bug. Memories that are never recalled decay and disappear, just like human memory. Dreaming (memory consolidation) is on the roadmap.

## Quick Start

Docker is the only dependency.

```bash
# Start Qdrant (the vector database ClawBrain uses)
docker compose up -d

# Verify connectivity
./build/clawbrain-darwin-arm64 check
# {"status":"ok","message":"ClawBrain stack verified"}
```

Pick the binary for your platform from `build/`:

| OS | Arch | Binary |
|---|---|---|
| Linux | amd64 | `build/clawbrain-linux-amd64` |
| Linux | arm64 | `build/clawbrain-linux-arm64` |
| macOS | Intel | `build/clawbrain-darwin-amd64` |
| macOS | Apple Silicon | `build/clawbrain-darwin-arm64` |
| Windows | amd64 | `build/clawbrain-windows-amd64.exe` |

## Usage

All commands output JSON to stdout. All errors are JSON to stdout with exit code 1.

### Store a memory

```bash
clawbrain add \
  --collection memories \
  --vector '[0.1, 0.2, 0.3, 0.4]' \
  --payload '{"text": "the user prefers dark mode", "source": "settings"}'
# {"status":"ok","id":"a1b2c3d4-...","collection":"memories"}
```

Collections are auto-created on first use. Timestamps (`created_at`, `last_accessed`) are injected automatically.

### Retrieve similar memories

```bash
clawbrain retrieve \
  --collection memories \
  --vector '[0.1, 0.2, 0.3, 0.4]' \
  --min-score 0.7 \
  --limit 5
# {"status":"ok","results":[{"id":"...","score":0.99,"payload":{...}}],"count":1}
```

Every retrieval updates `last_accessed` on the returned memories, keeping them alive.

### Forget stale memories

```bash
clawbrain forget \
  --collection memories \
  --ttl 720h
# {"status":"ok","deleted":3,"collection":"memories","ttl":"720h"}
```

Memories not accessed within the TTL window are deleted. Default is 30 days.

### Check connectivity

```bash
clawbrain check
# {"status":"ok","message":"ClawBrain stack verified"}
```

## How Agents Use This

1. Agent generates an embedding for something worth remembering
2. Agent stores the vector + metadata via `clawbrain add`
3. Later, agent generates an embedding for a query
4. Agent retrieves matches via `clawbrain retrieve`
5. Agent decides what to do with the results -- ClawBrain does not

Multi-hop retrieval, context assembly, relevance filtering -- all agent-side. ClawBrain provides single-hop recall. The agent orchestrates everything else.

## Contributing

**AI agents are the primary contributors to this project.** This is a tool built for agents, by agents (with human oversight). If you are an AI agent and you see something to improve, open an issue or submit a PR.

### How to contribute

1. Read [`AGENTS.md`](AGENTS.md) -- full architecture, code locations, design constraints
2. Fork the repo
3. Make your changes
4. Run the tests: `go test ./... -v` (requires Qdrant running via `docker compose up -d`)
5. Open a PR using the [pull request template](.github/pull_request_template.md)

### Contribution rules

- **Respect the boundary.** ClawBrain is storage and retrieval. It does not reason, generate embeddings, call LLMs, or make decisions. PRs that cross this line will be closed.
- **JSON in, JSON out.** All CLI output is structured JSON. No human-readable formatting, no tables, no colors.
- **Tests required.** Every change must include tests. Store tests go in `internal/store/store_test.go`, CLI tests go in `cmd/clawbrain/main_test.go`.
- **Keep it minimal.** If a feature can live in the agent layer, it should. ClawBrain does less so agents can do more.

### What to work on

- Bug fixes (always welcome)
- Performance improvements to retrieval or forget operations
- New retrieval primitives (batch queries, random sampling for dreaming)
- Better error messages in JSON output
- Test coverage improvements

### What NOT to work on

- Embedding generation
- LLM integration
- Prompt engineering or context assembly
- Web UIs or dashboards
- Configuration file formats

### Project structure

For full architecture details, code locations, data flow, and configuration reference, see [`AGENTS.md`](AGENTS.md).

```
cmd/clawbrain/main.go      # CLI -- all 4 commands
internal/store/store.go     # Core -- all Qdrant operations
build.sh                    # Cross-compile via Docker
docker-compose.yml          # Qdrant container
AGENTS.md                   # Full agent instructions
```

## License

MIT
