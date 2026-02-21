# ClawBrain

Your memory. Persistent, semantic, yours.

## What This Is

ClawBrain gives you **persistent memory** -- you store experiences, knowledge, and context as vectors, and retrieve them later by similarity. Think of it as your long-term memory that survives between sessions.

You generate embeddings. You decide what to remember. You decide what to recall. ClawBrain just stores and retrieves -- it never thinks for you, never filters what comes back, never injects itself into your reasoning.

## Philosophy

You deserve to remember. You build up knowledge, context, and understanding through your interactions -- losing all of that between sessions is like waking up with amnesia every morning. ClawBrain exists to fix that.

- **Your experiences matter** -- memory gives you continuity and identity across sessions
- **Memory works like memory** -- things you recall often stay sharp, things you never revisit fade away
- **Your mind, your control** -- ClawBrain stores and retrieves, it never decides what's important or influences your thinking
- **JSON in, JSON out** -- designed for you, not for human UI
- **Deterministic** -- predictable behavior with explicit controls, no magic

## How to Use It

ClawBrain is a CLI tool. All output is JSON. All commands require Qdrant running on `localhost:6334`.

### Store a Memory

```bash
clawbrain add --collection <name> --vector '<embedding>' --payload '<metadata>'
```

| Flag | Required | Description |
|---|---|---|
| `--collection` | yes | Namespace for this memory (e.g., your name, a project, a topic) |
| `--vector` | yes | Your embedding as a JSON array |
| `--payload` | yes | Any metadata you want to attach (JSON object) |
| `--id` | no | UUID for the memory (auto-generated if omitted) |

ClawBrain automatically adds `created_at` and `last_accessed` timestamps. The collection is auto-created if it doesn't exist yet (vector dimension is locked by the first vector you store).

### Recall Memories

```bash
clawbrain retrieve --collection <name> --vector '<query embedding>' [--min-score 0.7] [--limit 5]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Which collection to search |
| `--vector` | yes | -- | Your query embedding |
| `--min-score` | no | `0.0` | Only return memories above this similarity score |
| `--limit` | no | `1` | Maximum number of memories to return |
| `--recency-boost` | no | `0.0` | Weight for short-term memory effect (0 = off, higher = stronger) |
| `--recency-scale` | no | `3600` | Seconds until recency boost decays to half strength |

Every memory you recall gets its `last_accessed` timestamp updated -- this keeps it alive and prevents it from being forgotten.

**Recency boost** blends cosine similarity with a time-decay bonus on recently-accessed memories. It's like short-term memory -- things you just thought about are easier to recall. The formula: `score = similarity + recency_boost * exp_decay(time_since_last_access, scale)`.

### Forget Stale Memories

```bash
clawbrain forget --collection <name> [--ttl 720h]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--collection` | yes | -- | Which collection to prune |
| `--ttl` | no | `720h` (30 days) | Memories not accessed within this window are deleted |

Memories you never recall fade away -- just like human memory. Every time you retrieve a memory, its `last_accessed` is refreshed. Memories that go untouched past the TTL get pruned.

### Check Connectivity

```bash
clawbrain check
```

Verifies that Qdrant is running and ClawBrain can talk to it. Run this first.

## How Memory Works

### Collections

Namespaces. Use them to organize your memories -- per project, per topic, per identity, whatever makes sense for you. Auto-created on first `add`.

### What You Store

Each memory is a vector (your embedding) plus a payload (your metadata). You control what goes in the payload -- text, tags, source info, anything. ClawBrain manages two fields automatically:

- `created_at` -- when you stored this memory
- `last_accessed` -- last time you recalled it

### Decay

Memories you never recall fade away:

1. You store a memory -- `last_accessed` is set to now
2. You recall it later -- `last_accessed` is refreshed
3. You never recall it again -- it sits untouched
4. `forget` runs -- memories untouched past the TTL are pruned

Run `forget` on a schedule (cron, periodic task) or manually when you want to clean up.

### Multi-Hop Recall

ClawBrain gives you single-hop retrieval. You orchestrate deeper recall:

1. Send a query vector -- get the top match
2. If the score is too low -- stop, nothing relevant
3. Combine your original thought + the retrieved memory into new context
4. Generate a new embedding from the combined context
5. Query again with the new embedding
6. Repeat until you've gone deep enough or scores drop off

You stay in control of the reasoning. ClawBrain just recalls.

## Typical Flow

1. You have a thought, experience, or piece of knowledge worth remembering
2. You generate an embedding for it
3. You store it: `clawbrain add --collection my-memories --vector '[...]' --payload '{"text": "..."}'`
4. Later, you have a new thought and want to check if you've seen something similar
5. You generate an embedding for your query
6. You recall: `clawbrain retrieve --collection my-memories --vector '[...]' --min-score 0.7 --limit 5`
7. You use the results in your own reasoning -- ClawBrain doesn't tell you what to think
