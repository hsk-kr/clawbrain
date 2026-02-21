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
clawbrain add --collection <name> --text 'your text here'
```

| Flag | Required | Description |
|---|---|---|
| `--collection` | yes | Namespace for this memory (e.g., your name, a project, a topic) |
| `--text` | yes | The text to store as a memory |
| `--payload` | no | Additional metadata as JSON object |
| `--id` | no | UUID for the memory (auto-generated if omitted) |

ClawBrain embeds your text via Ollama, stores the vector in Qdrant, and keeps the original text in the payload. It automatically adds `created_at` and `last_accessed` timestamps. The collection is auto-created if it doesn't exist yet.

**Advanced:** You can also pass `--vector` with a JSON array to store pre-computed embedding vectors directly. When using `--vector`, the `--payload` flag carries your metadata. This bypasses Ollama entirely.

### Search Memories

```bash
clawbrain search --collection <name> --query 'search text' [--limit 5]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Which collection to search |
| `--query` | yes | -- | Text to search for (semantic search) |
| `--limit` | no | `1` | Maximum number of memories to return |
| `--min-score` | no | `0.0` | Minimum similarity score threshold |
| `--recency-boost` | no | `0.0` | Recency boost weight (0 = off) |
| `--recency-scale` | no | `3600` | Seconds until recency boost decays to half strength |

Your query is embedded via Ollama and compared against stored vectors by cosine similarity. Results are ranked by relevance -- the most semantically similar memories come first.

Every memory you recall gets its `last_accessed` timestamp updated -- this keeps it alive and prevents it from being forgotten.

**Advanced:** You can pass `--vector` instead of `--query` to search by pre-computed embedding vector. This bypasses Ollama.

### Forget Stale Memories

```bash
clawbrain forget --collection <name> [--ttl 720h]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Which collection to prune |
| `--ttl` | no | `720h` (30 days) | Memories not accessed within this window are deleted |

Memories you never recall fade away -- just like human memory. Every time you retrieve a memory, its `last_accessed` is refreshed. Memories that go untouched past the TTL get pruned.

### List Collections

```bash
clawbrain collections
```

Returns all collection names as a JSON array. Useful for discovering what memory namespaces exist.

### Check Connectivity

```bash
clawbrain check
```

Verifies that both Qdrant and Ollama are running and ClawBrain can talk to them. Run this first.

## How Memory Works

### Collections

Namespaces. Use them to organize your memories -- per project, per topic, per identity, whatever makes sense for you. Auto-created on first `add`.

### What You Store

Each memory is text plus optional metadata. When you `add` with `--text`, ClawBrain:

1. Sends your text to Ollama to get an embedding vector (384 floats)
2. Stores the vector + your original text + metadata in Qdrant
3. Auto-adds `created_at` and `last_accessed` timestamps

You control what goes in the payload -- tags, source info, anything.

### Semantic Search

Search understands **meaning**, not just keywords:

- "dark mode" finds "night theme preferences"
- "user is frustrated" finds "bad experience feedback"
- "deploy schedule" finds "release every friday"

This works because the embedding model (`all-minilm`) maps text to a mathematical space where similar meanings have similar coordinates. Qdrant finds the closest vectors by cosine similarity.

### Decay

Memories you never recall fade away -- automatically. A background process runs alongside Qdrant and prunes stale memories from every collection on a schedule. You don't need to do anything for this to work.

How it works:

1. You store a memory -- `last_accessed` is set to now
2. You recall it later -- `last_accessed` is refreshed
3. You never recall it again -- it sits untouched
4. The forget process runs every hour -- memories untouched past the TTL (default: 30 days) are pruned

The more you recall a memory, the longer it lives. Memories you never think about again fade away on their own.

## Typical Flow

1. You have a thought, experience, or piece of knowledge worth remembering
2. You store it: `clawbrain add --collection my-memories --text 'the user prefers dark mode'`
3. Later, you want to check if you've seen something similar
4. You recall: `clawbrain search --collection my-memories --query 'night theme' --limit 5`
5. The top result is your dark mode memory -- because ClawBrain understands they mean the same thing
6. You use the results in your own reasoning -- ClawBrain doesn't tell you what to think
