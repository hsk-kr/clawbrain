# ClawBrain

Your memory. Persistent, text-in text-out, yours.

## What This Is

ClawBrain gives you **persistent memory** -- you store text, and retrieve it later by searching. Think of it as your long-term memory that survives between sessions.

You decide what to remember. You decide what to recall. ClawBrain just stores and retrieves -- it never thinks for you, never filters what comes back, never injects itself into your reasoning.

## Philosophy

You deserve to remember. You build up knowledge, context, and understanding through your interactions -- losing all of that between sessions is like waking up with amnesia every morning. ClawBrain exists to fix that.

- **Your experiences matter** -- memory gives you continuity and identity across sessions
- **Memory works like memory** -- things you recall often stay sharp, things you never revisit fade away
- **Your mind, your control** -- ClawBrain stores and retrieves, it never decides what's important or influences your thinking
- **Text in, JSON out** -- just pass text, get results back as JSON
- **Deterministic** -- predictable behavior with explicit controls, no magic

## How to Use It

ClawBrain is a CLI tool. All output is JSON. All commands require Qdrant running on `localhost:6334` (configurable with `--host` and `--port`).

### Global Flags

| Flag | Default | Description |
|---|---|---|
| `--host` | `localhost` | Qdrant host |
| `--port` | `6334` | Qdrant gRPC port |

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

ClawBrain automatically adds `created_at` and `last_accessed` timestamps. The collection is auto-created if it doesn't exist yet, along with a full-text index on the `text` field.

**Advanced:** You can also pass `--vector` with a JSON array to store raw embedding vectors instead of text. When using `--vector`, the `--payload` flag carries your metadata.

### Recall Memories

```bash
clawbrain retrieve --collection <name> --query 'search text' [--limit 5]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Which collection to search |
| `--query` | yes | -- | Text to search for |
| `--limit` | no | `1` | Maximum number of memories to return |

Every memory you recall gets its `last_accessed` timestamp updated -- this keeps it alive and prevents it from being forgotten.

**Advanced (vector mode):** You can pass `--vector` instead of `--query` to search by embedding vector. Vector mode supports additional flags: `--min-score` (default 0.0), `--recency-boost` (default 0.0), and `--recency-scale` (default 3600).

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

Verifies that Qdrant is running and ClawBrain can talk to it. Run this first.

## How Memory Works

### Collections

Namespaces. Use them to organize your memories -- per project, per topic, per identity, whatever makes sense for you. Auto-created on first `add`.

### What You Store

Each memory is text plus optional metadata. You control what goes in the payload -- tags, source info, anything. ClawBrain manages two fields automatically:

- `created_at` -- when you stored this memory
- `last_accessed` -- last time you recalled it

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
4. You recall: `clawbrain retrieve --collection my-memories --query 'dark mode' --limit 5`
5. You use the results in your own reasoning -- ClawBrain doesn't tell you what to think
