# Loom Enterprise Runtime

Loom is an autonomous, high-performance multi-model enterprise runtime and inference engine. It seamlessly orchestrates specialist models, manages long-term long-context RAG memory, performs online self-supervised learning, streams real-time dataflow telemetry to external visualizers (RabbitMQ), and recovers automatically from execution errors via model swapping and tool retries.

---

## Key Enterprise Features

### 1. Multi-Model Pipeline & Agentic Chaining (`chain_request`)
When a primary general-purpose model encounters a task requiring capabilities outside its scope (e.g. vision analysis, complex code generation, or advanced math), it natively triggers `chain_request`. Loom dynamically loads the best-matched specialist model into VRAM, executes the subtask, and synthesizes the results.

### 2. High-Performance Enterprise RAG Engine
- **Multi-Strategy Document Chunking:** Recursive character splitting, sliding windows, and Markdown header section boundary splitting.
- **Ultra-Fast Hybrid Search:** Combines dense vector embeddings with an in-memory BM25 sparse keyword index using Reciprocal Rank Fusion (RRF) for $<1\text{ ms}$ lookup latencies.
- **Source Attribution & Citations:** Injected prompt contexts feature provenance citations `(Source: document.pdf | Page: 4 | Lines: 12-45)`.
- **Self-Modifying Memory:** AI models use `update_memory` and `pin_memory` to update and prioritize long-term memory facts.

### 3. Automatic Error Fallback & Model Swapping
- **Failover Recovery:** Traps model completion failures or OOM errors and automatically swaps execution to alternative local models.
- **Tool Resilience:** Retries failed tool calls and returns graceful structured payloads to prevent pipeline crashes.

### 4. Real-Time Online Self-Supervised Learning (SSL) & Dataset Export
- **Online SSL Engine:** Non-blocking contrastive gradient estimation computes loss metrics during active conversation turns.
- **Dataset Exporter:** Run `loom export-data` to extract structured JSONL datasets ready for offline model pre-training or fine-tuning.

### 5. RabbitMQ Dataflow Telemetry & Visualization
- Streams real-time pipeline events (prompts, tool calls, RAG lookups, SSL steps) to external RabbitMQ/AMQP visualization brokers asynchronously without blocking inference path.

### 6. Zero-Copy Shared Memory (SHM) & Lock-Free IPC
- Employs POSIX `mmap` shared memory (`/dev/shm`) and lock-free atomic ring buffers (`ModuleDataBus`) to pass tensor references across backend modules with zero memory copy allocations.

---

## Configuration (`~/.loom/server.json`)

All configuration is centralized in `~/.loom/server.json`. If missing on startup, Loom automatically creates the file with complete default values:

```json
{
  "memory": {
    "enabled": true,
    "db_path": "/home/user/.loom/memory.lance",
    "embedding_model": "nomic-embed-text",
    "default_model": "qwen2.5-7b:latest",
    "log_path": "/home/user/.loom/server.log",
    "rabbitmq_enabled": false,
    "rabbitmq_url": "http://localhost:15672/api/exchanges/%2F/amq.default/publish",
    "heartbeat_interval_seconds": 30,
    "host": "127.0.0.1:11434",
    "models_dir": "/home/user/.loom/models",
    "debug": false,
    "chain_enabled": true,
    "chain_max_steps": 10,
    "top_k": 20,
    "similarity_threshold": 0.65,
    "importance_threshold": 0.3,
    "decay_rate": 0.01,
    "cache_size": 10000,
    "cache_max_cost": 67108864,
    "worker_count": 4,
    "max_prompt_memories": 10,
    "max_prompt_tokens": 2048
  }
}
```

---

## Building & Running

### Full Build (C++ & Go Native Libraries)
```bash
cmake -B build .
cmake --build build --parallel 8
./loom serve
```

### Quick Go Iteration
```bash
export CGO_CFLAGS="-I$(pwd)/include"
export CGO_LDFLAGS="$(pwd)/lib/linux_amd64/liblancedb_go.a -lm"
go build .
./loom serve
```

### Export Dataset for Fine-Tuning
```bash
loom export-data my_training_dataset.jsonl
```
