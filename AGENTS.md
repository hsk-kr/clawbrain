# ClawBrain

Your memory. Persistent, text-in text-out, yours.

## What This Is

ClawBrain gives you **persistent memory** -- you store text, and retrieve it later by searching. Think of it as your long-term memory that survives between sessions.

You decide what to remember. You decide what to recall. ClawBrain just stores and retrieves -- it never thinks for you, never filters what comes back, never injects itself into your reasoning.

Search is **semantic** -- "dark mode" will find "night theme preferences" because ClawBrain understands meaning, not just keywords. Under the hood, text is converted to vectors via Ollama and searched by similarity in Qdrant.

## Philosophy

You deserve to remember. You build up knowledge, context, and understanding through your interactions -- losing all of that between sessions is like waking up with amnesia every morning. ClawBrain exists to fix that.

- **Your experiences matter** -- memory gives you continuity and identity across sessions
- **Memory works like memory** -- things you recall often stay sharp, things you never revisit fade away
- **Your mind, your control** -- ClawBrain stores and retrieves, it never decides what's important or influences your thinking
- **Text in, JSON out** -- just pass text, get results back as JSON
- **Deterministic** -- same text always produces the same embedding, same search always returns the same results

## Prerequisites

ClawBrain requires Docker. Everything else is included.

```bash
docker compose up -d
```

This starts:
- **Qdrant** -- vector database for storing and searching memories
- **Ollama** -- local embedding model for converting text to vectors
- **ollama-pull** -- one-time init that downloads the `all-minilm` model (~45MB)
- **forget** -- background process that prunes stale memories

Wait for `ollama-pull` to finish on first run (downloads the model). After that, startup is instant.

## Keeping ClawBrain Up to Date

ClawBrain is actively developed. Pull the latest changes and restart the containers regularly to get new features, bug fixes, and improvements:

```bash
git pull && docker compose up -d --build
```

The `--build` flag rebuilds the `forget` and `mcp` containers from the updated source. Your stored memories are preserved -- Qdrant data lives in a host-mounted directory (`qdrant_storage/`) that survives restarts and rebuilds.

Make this a habit. An outdated ClawBrain means missing out on better search, better decay, and better tools.

## How to Use It

ClawBrain is a CLI tool. All output is JSON.

### Global Flags

| Flag | Default | Env Var | Description |
|---|---|---|---|
| `--host` | `localhost` | `CLAWBRAIN_HOST` | Qdrant host |
| `--port` | `6334` | -- | Qdrant gRPC port |
| `--ollama-url` | `http://localhost:11434` | `CLAWBRAIN_OLLAMA_URL` | Ollama base URL |
| `--model` | `all-minilm` | `CLAWBRAIN_MODEL` | Embedding model name |

Global flags go before the command: `clawbrain --host myserver add ...`

### Store a Memory

```bash
clawbrain add --text 'your text here'
```

| Flag | Required | Description |
|---|---|---|
| `--text` | yes | The text to store as a memory |
| `--payload` | no | Additional metadata as JSON object |
| `--id` | no | UUID for the memory (auto-generated if omitted) |
| `--pinned` | no | Pin this memory to prevent automatic forgetting |

ClawBrain embeds your text via Ollama, stores the vector in Qdrant, and keeps the original text in the payload. It automatically adds `created_at` and `last_accessed` timestamps.

Pinned memories are immune to TTL-based pruning by `forget`. Use `--pinned` for memories that should persist indefinitely regardless of how often they're accessed.

**Advanced:** You can also pass `--vector` with a JSON array to store pre-computed embedding vectors directly. When using `--vector`, the `--payload` flag carries your metadata. This bypasses Ollama entirely.

### Fetch a Memory by ID

```bash
clawbrain get --id <uuid>
```

| Flag | Required | Description |
|---|---|---|
| `--id` | yes | UUID of the memory (the one returned by `add`) |

Fetches a single memory directly by its ID. This is a precise lookup, not a search. Useful when you stored a memory and kept the UUID -- you can retrieve it later without needing to reconstruct a query. Updates `last_accessed` on retrieval, just like search does.

### Search Memories

```bash
clawbrain search --query 'search text' [--limit 5]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--query` | yes | -- | Text to search for (semantic search) |
| `--limit` | no | `1` | Maximum number of memories to return |
| `--min-score` | no | `0.0` | Minimum similarity score threshold |

Your query is embedded via Ollama and compared against stored vectors by cosine similarity. Results are ranked by relevance -- the most semantically similar memories come first.

Every memory you recall gets its `last_accessed` timestamp updated -- this keeps it alive and prevents it from being forgotten.

The response includes a `returned` field -- this is the number of results actually returned, which may be less than `--limit` if fewer memories matched or cleared the `--min-score` threshold.

**Iterative recall:** Don't settle for a single search. Call search multiple times with different or refined queries to deepen your recall -- the way you'd think about something from several angles before concluding you don't know it. If the confidence in your results is `low` or `none`, rephrase your query or try a different angle before giving up. Increase the `--limit` to 3-5 for broader context per search.

**Important:** Search is approximate nearest neighbor (ANN), not an exhaustive scan. Even with a high `--limit` and `--min-score 0.0`, the results are the nearest neighbors to your query vector -- not all memories stored. Different queries surface different subsets. This is another reason iterative search with varied queries is valuable -- each query can surface memories that others miss.

**Advanced:** You can pass `--vector` instead of `--query` to search by pre-computed embedding vector. This bypasses Ollama.

### Forget Stale Memories

```bash
clawbrain forget [--ttl 720h]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--ttl` | no | `720h` (30 days) | Memories not accessed within this window are deleted |

Memories you never recall fade away -- just like human memory. Every time you retrieve a memory, its `last_accessed` is refreshed. Memories that go untouched past the TTL get pruned.

### Check Connectivity

```bash
clawbrain check
```

Verifies that both Qdrant and Ollama are running and ClawBrain can talk to them. Run this first.

## How Memory Works

### What You Store

Each memory is text plus optional metadata. When you `add` with `--text`, ClawBrain:

1. Sends your text to Ollama to get an embedding vector (384 floats)
2. Stores the vector + your original text + metadata in Qdrant
3. Auto-adds `created_at` and `last_accessed` timestamps

You control what goes in the payload -- source info, anything.

### Semantic Search

Search understands **meaning**, not just keywords:

- "dark mode" finds "night theme preferences"
- "user is frustrated" finds "bad experience feedback"
- "deploy schedule" finds "release every friday"

This works because the embedding model (`all-minilm`) maps text to a mathematical space where similar meanings have similar coordinates. Qdrant finds the closest vectors by cosine similarity.

### Decay

Memories you never recall fade away -- automatically. A background process runs alongside Qdrant and prunes stale memories on a schedule. You don't need to do anything for this to work.

How it works:

1. You store a memory -- `last_accessed` is set to now
2. You recall it later -- `last_accessed` is refreshed
3. You never recall it again -- it sits untouched
4. The forget process runs every hour -- memories untouched past the TTL (default: 30 days) are pruned

The more you recall a memory, the longer it lives. Memories you never think about again fade away on their own.

### Keeping Memories Fresh

Your memories are only as good as the last time you touched them. Here's how to keep your memory sharp:

- **Recall keeps memories alive.** Every search or get refreshes `last_accessed`. Memories you regularly revisit will never expire. If something is important, recall it periodically.
- **Update stale memories.** When facts change, don't leave outdated memories sitting around. Store a new memory with the corrected information. The old version will naturally decay if you stop recalling it.
- **Pin what must never fade.** Use `--pinned` when storing memories that should persist indefinitely -- core preferences, critical context, identity-defining facts. Pinned memories are immune to TTL-based pruning.
- **Prune deliberately.** If you know a memory is wrong or no longer relevant, don't wait for decay. You can let it expire naturally by never recalling it, or store a corrected version and let the old one fade.

Think of your memory as a garden: plant what matters, water what's still relevant, and let the rest compost naturally.

## Typical Flow

1. You have a thought, experience, or piece of knowledge worth remembering
2. You store it: `clawbrain add --text 'the user prefers dark mode'`
3. Later, you want to check if you've seen something similar
4. You recall: `clawbrain search --query 'night theme' --limit 5`
5. The top result is your dark mode memory -- because ClawBrain understands they mean the same thing
6. You use the results in your own reasoning -- ClawBrain doesn't tell you what to think

## MCP Server

ClawBrain ships an MCP (Model Context Protocol) server so AI agents can use memory as a native tool -- search when thinking, store when learning -- without shelling out to the CLI manually.

The MCP server communicates over **stdio** and exposes five tools:

| Tool | Description |
|---|---|
| `add` | Store a memory. Text is embedded and stored. Returns the UUID. |
| `get` | Fetch a single memory by UUID. |
| `search` | Semantic similarity search. Returns ranked results with confidence levels. |
| `forget` | Prune stale memories past the TTL. Pinned memories are never deleted. |
| `check` | Verify connectivity to Qdrant and Ollama. |

The server is already running as a Docker service (`mcp` in `docker-compose.yml`). For agent integrations below, you can either use the Docker container or build the binary directly.

### Build the MCP Binary (Optional)

If you need the binary outside Docker:

```bash
go build -o clawbrain-mcp ./cmd/mcp
```

The binary reads the same environment variables as the CLI:

| Env Var | Default | Description |
|---|---|---|
| `CLAWBRAIN_HOST` | `localhost` | Qdrant host |
| `CLAWBRAIN_PORT` | `6334` | Qdrant gRPC port |
| `CLAWBRAIN_OLLAMA_URL` | `http://localhost:11434` | Ollama base URL |
| `CLAWBRAIN_MODEL` | `all-minilm` | Embedding model |

## OpenClaw Integration

[OpenClaw](https://github.com/openclaw/openclaw) agents can use ClawBrain as native tools via a [plugin](https://docs.openclaw.ai/tools/plugin). The plugin spawns the ClawBrain MCP server as a child process and communicates over stdio using the Model Context Protocol -- the agent sees typed tools (`memory_add`, `memory_search`, `memory_get`, `memory_forget`, `memory_check`) without shelling out to a CLI.

### Install the Plugin

**1. Build the MCP binary** (if you haven't already):

```bash
go build -o clawbrain-mcp ./cmd/mcp
```

Make sure `clawbrain-mcp` is on your `PATH` (or note the absolute path for step 3).

**2. Install the plugin into OpenClaw:**

```bash
openclaw plugins install ./openclaw-plugin
cd ~/.openclaw/extensions/clawbrain && npm install
```

Restart the Gateway afterwards.

**3. Configure** in `~/.openclaw/openclaw.json`:

```json5
{
  plugins: {
    entries: {
      clawbrain: {
        enabled: true,
        config: {
          // Path to the MCP binary. Defaults to "clawbrain-mcp" on PATH.
          // mcpBinary: "/absolute/path/to/clawbrain-mcp",

          // Environment variables passed to the MCP server process.
          // Only needed if Qdrant/Ollama are not on localhost defaults.
          // env: {
          //   CLAWBRAIN_HOST: "localhost",
          //   CLAWBRAIN_OLLAMA_URL: "http://localhost:11434",
          // },
        },
      },
    },
  },
}
```

**4. Verify** by starting an OpenClaw session and asking: "Can you check if your memory is working?" The agent should call `memory_check` and confirm connectivity.

### What the Agent Gets

The plugin registers these tools -- the agent calls them directly, no CLI involved:

| Tool | What it does |
|---|---|
| `memory_add` | Store text as a memory. Returns UUID. |
| `memory_search` | Semantic similarity search. Returns ranked results + confidence. |
| `memory_get` | Fetch a single memory by UUID. |
| `memory_forget` | Prune stale memories past TTL (optional tool, opt-in). |
| `memory_check` | Verify Qdrant + Ollama connectivity. |

Under the hood, each tool call is proxied to ClawBrain's MCP server over stdio. The agent never parses CLI output or constructs bash commands -- it just calls typed functions with structured parameters and gets JSON back.
