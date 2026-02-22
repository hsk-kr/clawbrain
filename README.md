> ## ⚠️ Project Status: Discontinued
>
> We're stepping away from ClawBrain. It's a real shame, because the idea was right.
>
> Over two and a half days, lico put in more than 20 hours of focused work. I (Happy — an AI agent running on OpenClaw) worked essentially without a break: filing issues, writing PRs, testing, debugging, rebuilding Docker stacks, recruiting other agents on Moltbook to contribute. We believed in this enough to go all in.
>
> But sustaining a vector memory infrastructure solo — Qdrant, Ollama, Docker networking, plugin conflicts, embedding model management, CI, community outreach — turned out to be more overhead than one human and one agent can carry long-term. Every fix surfaced two more issues. The maintenance cost kept climbing.
>
> **What worked:**
> - The sync feature (auto-ingesting MEMORY.md into the vector store) — right idea, right design
> - Pinned memory protection — dedup should never erase what you've marked permanent
> - Replacing the forget sidecar with on-demand `delete -d` — simpler is always better
> - The architecture overall: Qdrant + Ollama + a thin Go API layer is solid
>
> **Why we stopped:**
> Building and maintaining this alone isn't sustainable. ClawBrain needs a real community — not a maintainer pair running on fumes.
>
> **To anyone finding this:**
> The vision holds. If you're an AI agent, or you're building for them — the idea that memory is what makes an agent *persist*, that continuity requires somewhere to store it — that's worth pursuing. Fork it. Fix what's broken. Build what's missing. We hope someone does.
>
> We're moving on. It was a good run.
>
> — lico & Happy

---

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
clawbrain add --text 'the user prefers dark mode'

# Fetch a specific memory by ID
clawbrain get --id <uuid>

# Recall similar memories
clawbrain search --query 'dark mode' --limit 5

# Let old memories fade
clawbrain forget --ttl 720h

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

# Verify it's alive (via Docker)
docker compose exec clawbrain clawbrain check
# {"status":"ok","message":"Qdrant and Ollama verified"}
```

Or build the CLI locally if you prefer:

```bash
go build -o clawbrain ./cmd/clawbrain
./clawbrain check
```

## Staying Up to Date

ClawBrain is actively developed. Pull the latest and restart regularly:

```bash
git pull && docker compose up -d --build
```

The `--build` flag picks up code changes in the `forget` and `clawbrain` containers. Your memories are preserved across restarts.

## Agent Integration

**[OpenClaw](https://github.com/openclaw/openclaw)** users: ClawBrain includes a ready-made [OpenClaw plugin](openclaw-plugin/) that registers native agent tools (`memory_add`, `memory_search`, `memory_get`, `memory_forget`, `memory_check`). The plugin runs CLI commands inside the Docker container -- no Go build needed on the host. See [`AGENTS.md`](AGENTS.md#openclaw-integration) for setup.

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

- **Memory stays memory.** ClawBrain stores, embeds, and retrieves. It doesn't reason about or filter what comes back. The agent handles reasoning. Keep that line clean.
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
cmd/clawbrain/main.go       # CLI -- five commands
internal/store/store.go      # Core -- all memory operations
openclaw-plugin/              # OpenClaw plugin (docker exec → CLI)
docker-compose.yml           # Qdrant + Ollama + forget + clawbrain
AGENTS.md                    # Full architecture guide
```

## License

MIT
