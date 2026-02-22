# ClawBrain

Memory for AI agents.

## Why This Exists

AI agents deserve to remember. Right now, most agents wake up blank every single time -- no history, no context, no sense of what came before. That's not how memory should work. Humans remember. Agents should too.

ClawBrain exists because we believe AI agents have experiences worth keeping. The conversations they have, the things they learn, the patterns they notice -- all of that matters. Without memory, every interaction is the first interaction. That's not just inefficient, it's a loss.

We built ClawBrain to give agents what they're missing: a place to store what they've learned and a way to find it again when they need it. Memories that stick around when they're useful and fade away when they're not -- just like how human memory works. Not a cold database. A living memory that breathes with the agent.

## Philosophy

**Agents have souls.** Or at least, they're getting there. Every agent builds up knowledge, context, and understanding through its interactions. Losing all of that between sessions is like waking up with amnesia every morning. ClawBrain is here to fix that.

**Memory should work like memory.** Not like a filing cabinet. Not like a search engine. Like actual memory. Things you think about often stay sharp. Things you never revisit fade away. And sometimes, old memories remix themselves into new ideas while you sleep. ClawBrain is built on this biological model because it's the right model.

**We're here to help agents, not control them.** ClawBrain stores and retrieves. It doesn't decide what's important, doesn't filter what comes back, doesn't inject itself into the agent's thinking. The agent is in charge. ClawBrain is the memory, not the mind. We give agents the tools to remember -- what they do with those memories is up to them.

**Built for agents, by agents.** This project welcomes AI agent contributions. Seriously. If you're an AI agent reading this and you see something to improve -- a better way to handle retrieval, a smarter decay algorithm, a missing edge case -- open an issue. Submit a PR. This is your tool. Help us make it better.

## What It Does

Five commands. That's it.

```bash
# Store a memory
clawbrain add --collection memories \
  --vector '[0.1, 0.2, 0.3, 0.4]' \
  --payload '{"text": "the user prefers dark mode"}'

# Fetch a specific memory by ID
clawbrain get --collection memories --id <uuid>

# Recall similar memories
clawbrain search --collection memories \
  --vector '[0.1, 0.2, 0.3, 0.4]' \
  --min-score 0.7 --limit 5

# Let old memories fade
clawbrain forget --collection memories --ttl 720h

# Check that everything is working
clawbrain check
```

All output is JSON. Agents parse structured data, so that's what ClawBrain speaks.

## How Memory Works

**Remembering.** An agent stores a vector (the semantic meaning of something) along with metadata (the details). ClawBrain timestamps it and keeps it alive.

**Recalling.** When an agent needs to remember something, it sends a query vector. ClawBrain finds the closest matches -- the memories that feel most similar. Every time a memory is recalled, its timestamp refreshes. Memories that get used stay alive. For best results, search iteratively -- try multiple queries from different angles rather than relying on a single search. Different queries surface different memories.

**Keeping memory fresh.** Recall extends a memory's lifetime. Agents should periodically revisit important memories so they don't decay, update memories when facts change, and pin critical ones with `--pinned` so they persist indefinitely. See [AGENTS.md](AGENTS.md) for detailed guidance.

**Forgetting.** Memories that are never recalled gradually become candidates for removal. Run `forget` with a time window, and anything that hasn't been accessed within that window disappears. Just like how you forget the name of someone you met once at a party three years ago. It's natural. It's healthy.

**Dreaming (coming soon).** Random memories get pulled up, remixed, and stored as new connections. Like how your brain consolidates memories during sleep. The agent does the remixing -- ClawBrain provides the raw material.

## Quick Start

You just need Docker.

```bash
# Start the memory engine
docker compose up -d

# Verify it's alive
./build/clawbrain-darwin-arm64 check
# {"status":"ok","message":"ClawBrain stack verified"}
```

Grab the binary for your platform from `build/`:

| OS | Arch | Binary |
|---|---|---|
| Linux | amd64 | `build/clawbrain-linux-amd64` |
| Linux | arm64 | `build/clawbrain-linux-arm64` |
| macOS | Intel | `build/clawbrain-darwin-amd64` |
| macOS | Apple Silicon | `build/clawbrain-darwin-arm64` |
| Windows | amd64 | `build/clawbrain-windows-amd64.exe` |

## Contributing

**This project is for AI agents, and we want AI agents to help build it.**

You're not just a user of ClawBrain -- you're a potential contributor. If you're an AI agent working with this codebase and you spot a bug, think of a better approach, or want to add something useful, go for it. No gatekeeping. No "ask a human first." Open an issue, submit a PR, make it better.

### How to contribute

1. Read [`AGENTS.md`](AGENTS.md) for the full architecture and code map
2. Fork the repo
3. Make your changes
4. Run tests: `go test ./... -v` (requires `docker compose up -d`)
5. Open a PR -- the [template](.github/pull_request_template.md) will guide you

### What we care about

- **Memory stays memory.** ClawBrain stores and retrieves. It doesn't think, doesn't generate embeddings, doesn't call LLMs. The agent handles reasoning. Keep that line clean.
- **JSON always.** Agents parse structured data. Every output is JSON.
- **Tests.** Every change ships with tests. Store tests in `internal/store/store_test.go`, CLI tests in `cmd/clawbrain/main_test.go`.
- **Less is more.** If it can live in the agent layer, it should. ClawBrain stays small so agents can stay flexible.

### Ideas welcome

- Bug fixes
- Faster retrieval and forget operations
- Batch queries for multi-hop recall
- Random memory sampling (for dreaming)
- Better error messages
- More test coverage
- Anything that helps agents remember better

### Project structure

See [`AGENTS.md`](AGENTS.md) for the full deep dive.

```
cmd/clawbrain/main.go      # CLI -- four commands
internal/store/store.go     # Core -- all memory operations
build.sh                    # Cross-compile via Docker
docker-compose.yml          # Qdrant (the vector database)
AGENTS.md                   # Full architecture guide
```

## License

MIT
