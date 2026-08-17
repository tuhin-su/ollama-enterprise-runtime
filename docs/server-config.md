# Loom Server Configuration Guide (`~/.loom/server.json`)

Loom's behavior is controlled entirely by `~/.loom/server.json`. This file is loaded at startup and merged with built-in production defaults.

> [!NOTE]
> If `~/.loom/server.json` does not exist when Loom starts, the server **automatically creates it** with complete, production-ready default values.

---

## Complete Auto-Generated `server.json` Template

```json
{
  "host": "127.0.0.1:11434",
  "models_dir": "/home/user/.loom/models",
  "default_model": "",
  "log_path": "/home/user/.loom/server.log",
  "debug": false,
  "rabbitmq_enabled": false,
  "rabbitmq_url": "http://localhost:15672/api/exchanges/%2F/amq.default/publish",
  "heartbeat_interval_seconds": 30,
  "memory": {
    "enabled": false,
    "db_path": "/home/user/.loom/memory.lance",
    "embedding_model": "nomic-embed-text",
    "chain_enabled": true,
    "chain_max_steps": 10,
    "top_k": 20,
    "similarity_threshold": 0.65,
    "importance_threshold": 0.3,
    "decay_rate": 0.01,
    "cache_size": 10000,
    "cache_max_cost": 67108864,
    "cache_ttl": "5m0s",
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

---

## Detailed Option Reference

### 1. Server & Connectivity Settings

| Parameter | Type | Default | Description |
|---|---|---|---|
| `host` | `string` | `"127.0.0.1:11434"` | IP address and port Loom binds to. Use `"0.0.0.0:11434"` for external network access. |
| `models_dir` | `string` | `"~/.loom/models"` | Directory path where local model weights and manifests are stored. |
| `default_model` | `string` | `""` | Fallback model name when a request omits a model. |
| `log_path` | `string` | `"~/.loom/server.log"` | Path to the main server diagnostic log file. |
| `debug` | `boolean` | `false` | Enable verbose debug logging across model execution and tool routing. |

---

### 2. Autonomous Heartbeat & Telemetry

| Parameter | Type | Default | Description |
|---|---|---|---|
| `heartbeat_interval_seconds` | `integer` | `30` | Interval (in seconds) for the 24/7 background heartbeat monitor that checks loaded models for idle task execution or memory unloading. |
| `rabbitmq_enabled` | `boolean` | `false` | Activate real-time non-blocking event streaming to an external RabbitMQ broker. |
| `rabbitmq_url` | `string` | `"http://localhost:15672/..."` | AMQP / RabbitMQ HTTP Management API endpoint URL for dataflow telemetry. |

---

### 3. Enterprise RAG & Memory Subsystem (`memory`)

| Parameter | Type | Default | Description |
|---|---|---|---|
| `memory.enabled` | `boolean` | `false` | Enables long-term vector RAG memory middleware. |
| `memory.db_path` | `string` | `"~/.loom/memory.lance"` | LanceDB vector database storage directory. |
| `memory.embedding_model` | `string` | `"nomic-embed-text"` | Model used for vector embeddings. |
| `memory.chain_enabled` | `boolean` | `true` | Enables multi-model pipeline chaining (`chain_request`). |
| `memory.chain_max_steps` | `integer` | `10` | Maximum subtask steps allowed in a single model pipeline. |
| `memory.top_k` | `integer` | `20` | Candidate vector results fetched before RRF ranking pass. |
| `memory.similarity_threshold` | `float` | `0.65` | Minimum cosine similarity score for memory context inclusion. |
| `memory.importance_threshold` | `float` | `0.3` | Minimum importance score required to save a new memory. |
| `memory.worker_count` | `integer` | `4` | Number of background worker goroutines. |
| `memory.max_prompt_memories` | `integer` | `10` | Maximum memories injected into system prompt per chat turn. |
| `memory.max_prompt_tokens` | `integer` | `2048` | Maximum token budget spent on memory context injection. |

---

### 4. Memory Multi-Signal Ranking Weights (`memory.ranking`)

Loom uses a multi-signal scoring algorithm to rank relevant memories before prompt injection:

$$\text{Score} = w_{\text{sim}} \cdot \text{Similarity} + w_{\text{imp}} \cdot \text{Importance} + w_{\text{rec}} \cdot \text{Recency} + w_{\text{freq}} \cdot \text{Frequency} + w_{\text{pin}} \cdot \text{Pinned}$$

| Parameter | Type | Default | Description |
|---|---|---|---|
| `memory.ranking.similarity` | `float` | `0.4` | Weight given to vector semantic similarity. |
| `memory.ranking.importance` | `float` | `0.25` | Weight given to stored importance score. |
| `memory.ranking.recency` | `float` | `0.2` | Weight given to recency decay. |
| `memory.ranking.frequency` | `float` | `0.1` | Weight given to memory access count. |
| `memory.ranking.pinned` | `float` | `0.05` | Score boost for pinned memories. |
