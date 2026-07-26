# Native Long-Term Memory System

Ollama includes a built-in long-term memory system that lets models remember
users, projects, preferences, and past conversations across sessions — without
any external service, database, or wrapper application.

---

## How It Works

```
User sends message
        │
        ▼
┌─────────────────────────────┐
│   Memory Middleware         │
│  • Embeds the query         │
│  • Searches vector index    │  ← <100 ns (Ristretto cache hit)
│  • Injects top memories     │
│    into system prompt       │
└─────────────────────────────┘
        │
        ▼
┌─────────────────────────────┐
│   Ollama Model              │  ← sees personalised context
│   (inference)               │
└─────────────────────────────┘
        │
        ▼
┌─────────────────────────────┐
│   Background Worker         │
│  • Extracts new facts       │  ← async, zero latency impact
│  • Generates embeddings     │
│  • Persists to SQLite       │
└─────────────────────────────┘
```

---

## Quick Start

### 1. Pull an embedding model

The memory system uses Ollama's own `/api/embed` endpoint:

```bash
ollama pull nomic-embed-text
```

Any embedding model works. `nomic-embed-text` is recommended for its speed and
accuracy at 768 dimensions.

### 2. Enable memory in `~/.ollama/server.json`

```json
{
  "memory": {
    "enabled": true,
    "embedding_model": "nomic-embed-text"
  }
}
```

### 3. Start the server

```bash
ollama serve
```

That's it. Memory is now active for all local chat requests.

---

## Memory Types

| Type | What Gets Stored | Example |
|------|-----------------|---------|
| `user` | Personal facts | "My name is Tuhin", "I am a Go developer" |
| `project` | Project context | "Working on an API called Zephyr using Go" |
| `episodic` | Events | "Fixed the authentication bug", "Deployed v2" |
| `semantic` | General knowledge | Concepts discussed in conversations |
| `conversation` | Session summaries | Compressed summaries for long sessions |

---

## Configuration

All options live under the `"memory"` key in `~/.ollama/server.json`.

```json
{
  "memory": {
    "enabled": true,
    "db_path": "~/.ollama/memory.db",
    "embedding_model": "nomic-embed-text",
    "top_k": 20,
    "similarity_threshold": 0.65,
    "importance_threshold": 0.3,
    "decay_rate": 0.01,
    "cache_size": 10000,
    "cache_max_cost": 67108864,
    "cache_ttl": "5m",
    "worker_count": 4,
    "max_prompt_memories": 10,
    "max_prompt_tokens": 2048,
    "decay_interval_hours": 24,
    "archive_after_days": 90,
    "ranking": {
      "similarity": 0.4,
      "importance": 0.25,
      "recency": 0.2,
      "frequency": 0.1,
      "pinned": 0.05
    }
  }
}
```

### Options Reference

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `false` | Master switch. Must be `true` to activate. |
| `db_path` | `~/.ollama/memory.db` | Path to the SQLite database file. |
| `embedding_model` | `nomic-embed-text` | Ollama model used to generate embeddings. |
| `top_k` | `20` | Number of candidates retrieved from vector search before ranking. |
| `similarity_threshold` | `0.65` | Minimum cosine similarity (0–1) for a memory to be considered relevant. |
| `importance_threshold` | `0.3` | Minimum importance score for extracting and storing a new memory. |
| `decay_rate` | `0.01` | Daily importance decay rate. Higher = faster forgetting. |
| `cache_size` | `10000` | Maximum items in the in-memory Ristretto cache. |
| `cache_max_cost` | `67108864` | Maximum cache size in bytes (default 64 MiB). |
| `cache_ttl` | `5m` | Default cache entry lifetime (Go duration string). |
| `worker_count` | `4` | Background goroutines for async embedding and storage. |
| `max_prompt_memories` | `10` | Maximum number of memories injected per request. |
| `max_prompt_tokens` | `2048` | Token budget for injected memory context. |
| `decay_interval_hours` | `24` | How often the decay/archival worker runs. |
| `archive_after_days` | `90` | Archive memories not accessed for this many days. |

### Ranking Weights

The relevance score for each memory is computed as:

```
score = similarity  × 0.40   # cosine similarity to the query
      + importance  × 0.25   # stored importance (0–1)
      + recency     × 0.20   # exponential decay, 30-day half-life
      + frequency   × 0.10   # access count (log-scaled)
      + pinned      × 0.05   # pinned memories always score higher
```

All weights are tunable under `"ranking"` in the config.

---

## Storage

Memory is stored in a single SQLite file (default `~/.ollama/memory.db`):

- **WAL mode** — fast concurrent reads
- **Foreign keys enabled** — referential integrity
- **5 tables**: `memories`, `tags`, `memory_tags`, `conversations`, `summaries`
- **Embeddings** stored as binary little-endian float32 blobs

The file is fully portable — copy it between machines to transfer memories.

---

## Caching

An in-process [Ristretto](https://github.com/dgraph-io/ristretto) cache sits in
front of SQLite for hot-path performance:

| Cache Key | Contents | TTL |
|-----------|----------|-----|
| `mem:<id>` | Single memory object | 5 min |
| `search:<userID>:<query>` | Top-K search results | 2 min |
| `user:<userID>` | User-level metadata | 5 min |

Cache hits complete in **~80 ns**. SQLite queries take ~100 µs. The cache
ensures repeated queries in the same session have near-zero overhead.

---

## Memory Decay

Memories that are not accessed fade over time:

```
new_importance = importance × exp(−decay_rate × days_since_last_access)
```

- Default half-life: ~69 days (with `decay_rate = 0.01`)
- **Pinned memories** are immune to decay
- Frequently accessed memories get an access boost that counteracts decay
- Memories below `importance_threshold` are automatically archived (hidden from
  search but not deleted)

---

## Privacy

- All memories are stored **locally** in `~/.ollama/memory.db`
- Embeddings are generated by your local Ollama server — nothing is sent to the cloud
- The memory system is **off by default** (`"enabled": false`)
- To clear all memories: `rm ~/.ollama/memory.db`

---

## Limitations

- The vector index is an **exact brute-force search**. It is fast for up to
  ~100 000 memories but does not scale to millions. A future version will
  support HNSW indexing via [usearch](https://github.com/unum-cloud/usearch).
- Memory extraction uses **regex pattern matching**. It reliably detects common
  facts and events but may miss nuanced context that a dedicated extraction
  model would catch.
- Memories are keyed by **remote IP address** in single-user setups. For
  multi-user deployments, configure token auth (see
  [Server Configuration](server-config.md)) and update `memoryUserID()` in
  `server/memory_middleware.go` to use your authenticated user identifier.
