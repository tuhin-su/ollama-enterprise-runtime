# Loom Enterprise Architecture Overview

This document outlines the detailed architecture of Loom, focusing on **Agentic Model Chaining**, **Enterprise RAG Chunking & Hybrid Search**, **Automated Fallback Systems**, **Real-Time Online Self-Supervised Learning (SSL)**, **RabbitMQ Dataflow Telemetry**, and **Zero-Copy Shared Memory (SHM)**.

---

## 1. Core Model Chaining & Pipeline Orchestration (`server/model_chain.go`)

Loom enables a primary general-purpose orchestrator model to dynamically delegate subtasks (e.g. vision processing, coding, math) to specialized local models.

- **RAG Tool Manager**: To avoid Context Window bloat, native tools (`chain_request`, `system_tool`, `schedule_task`, `update_memory`) are registered inside a volatile LanceDB vector index (`ToolManager`).
- **Meta-Tool Search**: The model context starts slim, containing only `toolmanager.search`. Models query this meta-tool on demand.
- **Dynamic Model Resolution (`selectModelForTask`)**: Matches requested capabilities (`vision`, `code`) against model manifest descriptions and Modelfile `DESCRIPTION` keywords.

---

## 2. Enterprise RAG & Hybrid Search Subsystem (`server/memory/`)

Loom provides a multi-strategy RAG engine built for high-throughput and sub-millisecond retrieval:

- **Document Chunker (`chunker.go`)**: Supports **Recursive Character Splitting** (with fallback separators `\n\n`, `\n`, `. `, ` `), **Sliding Window Token Overlaps**, and **Markdown Header Boundary Splitting**.
- **BM25 In-Memory Index (`bm25.go`)**: Provides sub-millisecond sparse keyword lookups for exact term matching.
- **Reciprocal Rank Fusion (RRF)**: Merges dense vector cosine similarity scores with BM25 sparse search results without requiring neural cross-encoder passes.
- **Source Attribution**: Injects provenance headers `(Source: document.pdf | Page: 4 | Lines: 12-45)` into system prompt memory context.
- **Self-Modifying Memory (`assistant.go`)**: Exposes `update_memory` and `pin_memory` tools so models can self-manage, update, or prioritize long-term memory facts.

---

## 3. Automated Error Interception & Fallback System (`server/fallback.go`)

The `FallbackManager` provides automatic recovery across runtime inference failures and tool errors:

- **Model Swapping (`ExecuteWithModelFallback`)**: Intercepts model completion errors or OOMs and swaps execution to an alternative local model seamlessly.
- **Tool Call Retry & Graceful Payload (`ExecuteWithToolFallback`)**: Retries failed tool calls and returns graceful structured payloads to keep prompt streams intact.
- **Error Log Diagnostics (`get_error_logs`)**: Records error traces in an in-memory audit log accessible to the model for automated self-diagnosis.

---

## 4. Real-Time Online Self-Supervised Learning (SSL) (`server/memory/ssl.go`)

- **Online SSL Step (`LearnFromTurn`)**: Operates non-blockingly inside the background worker pool after completion streaming finishes, evaluating real-time contrastive loss ($L_{\text{contrastive}} = 1.0 - \text{CosineSimilarity}$).
- **Dataset Exporter (`cmd/export_data.go`)**: Exposes `loom export-data` to dump LanceDB interaction histories into structured JSONL training datasets for offline fine-tuning.

---

## 5. Telemetry & Shared Memory Optimization

- **RabbitMQ Telemetry (`server/rabbitmq.go`)**: Streams dataflow events (`chat_request`, `token_stream`, `tool_call`, `ssl_step`) to external RabbitMQ/AMQP visualization brokers.
- **POSIX Shared Memory (`server/memory/shm.go`)**: Uses `/dev/shm` POSIX `mmap` syscalls and unsafe pointer slice headers (`unsafe.Pointer`) to pass IPC tensor data with zero memory copies.
- **Lock-Free Cross-Module Bus (`server/memory/bus.go`)**: Atomic circular ring buffer (`ModuleDataBus`) enables sub-nanosecond module-to-module pointer passing without mutex locking.
