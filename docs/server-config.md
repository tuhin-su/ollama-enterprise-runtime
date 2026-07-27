# Server Configuration

Ollama's server behaviour can be tuned by creating `~/.ollama/server.json`.
The file is read at startup and merged with built-in defaults.

---

## Full Example

```json
{
  "api_token": "your-secret-token",
  "memory": {
    "enabled": true,
    "embedding_model": "nomic-embed-text"
  }
}
```

---

## Token Authentication

By default the Ollama API is **unauthenticated** and accessible to anyone who
can reach the server's port. When `api_token` is set, every API request must
include a matching `Authorization` header.

### Setting a token

Add `api_token` to `~/.ollama/server.json`:

```json
{
  "api_token": "my-super-secret-token"
}
```

Restart `ollama serve` for the change to take effect.

### How it works

- The server enforces `Authorization: Bearer <token>` on **all** endpoints
  except the health-check (`GET /` and `HEAD /`).
- The `ollama` CLI reads the same `server.json` file and **automatically**
  attaches the token to every local request — no manual configuration needed.
- Requests without a valid token receive `401 Unauthorized`.

### Using the API manually

```bash
# With token auth enabled
curl http://localhost:11434/api/chat \
  -H "Authorization: Bearer my-super-secret-token" \
  -d '{
    "model": "gemma4",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": false
  }'
```

### Security notes

- The token is stored in plain text in `~/.ollama/server.json`. Protect the
  file with appropriate filesystem permissions (`chmod 600`).
- Token auth is intended for **single-machine** or **LAN** deployments. For
  internet-facing deployments, put Ollama behind a reverse proxy with TLS.
- Cloud model requests to `ollama.com` continue to use SSH key authentication
  (`~/.ollama/id_ed25519`) regardless of the local token setting.

---

## Memory System

See [memory.md](memory.md) for the full reference.

### Minimal enable

```json
{
  "memory": {
    "enabled": true,
    "embedding_model": "nomic-embed-text"
  }
}
```

### Combined example

```json
{
  "api_token": "my-secret-token",
  "memory": {
    "enabled": true,
    "embedding_model": "nomic-embed-text",
    "top_k": 15,
    "max_prompt_memories": 8,
    "ranking": {
      "similarity": 0.5,
      "recency": 0.3,
      "importance": 0.1,
      "frequency": 0.05,
      "pinned": 0.05
    }
  }
}
```

---

## All Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `api_token` | string | `""` | Bearer token for API authentication. Empty = no auth. |
| `memory.enabled` | bool | `false` | Enable long-term memory system. |
| `memory.db_path` | string | `~/.ollama/memory.lance` | LanceDB database path. |
| `memory.embedding_model` | string | `nomic-embed-text` | Model for generating embeddings. |
| `memory.top_k` | int | `20` | Vector search candidates before ranking. |
| `memory.similarity_threshold` | float | `0.65` | Minimum cosine similarity (0–1). |
| `memory.importance_threshold` | float | `0.3` | Minimum importance to store a memory. |
| `memory.decay_rate` | float | `0.01` | Daily importance decay rate. |
| `memory.cache_size` | int | `10000` | Ristretto cache item count limit. |
| `memory.cache_max_cost` | int | `67108864` | Ristretto max cost in bytes (64 MiB). |
| `memory.cache_ttl` | duration | `"5m"` | Default cache TTL. |
| `memory.worker_count` | int | `4` | Background worker goroutines. |
| `memory.max_prompt_memories` | int | `10` | Max memories injected per request. |
| `memory.max_prompt_tokens` | int | `2048` | Token budget for memory context. |
| `memory.decay_interval_hours` | int | `24` | Hours between decay runs. |
| `memory.archive_after_days` | int | `90` | Days before archiving unused memories. |
| `memory.ranking.similarity` | float | `0.4` | Weight for vector similarity score. |
| `memory.ranking.importance` | float | `0.25` | Weight for stored importance. |
| `memory.ranking.recency` | float | `0.2` | Weight for recency decay score. |
| `memory.ranking.frequency` | float | `0.1` | Weight for access frequency. |
| `memory.ranking.pinned` | float | `0.05` | Boost for pinned memories. |
