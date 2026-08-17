# Engineering Loom: A Complete Journey from Static Inference Router to Autonomous AI Operating System

> **Author:** Tuhin | **Stack:** Go, LanceDB, LLaMA.cpp, WebSockets, Ristretto Cache
> **Repository:** `loom-master` | **Platform:** Linux x86-64

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Architecture Overview & Data Flow](#2-architecture-overview--data-flow)
3. [Feature 1 — Native Long-Term Memory System](#3-feature-1--native-long-term-memory-system)
4. [Feature 2 — RAG Tool Manager](#4-feature-2--rag-tool-manager)
5. [Feature 3 — Persistent WebSocket Tool Servers](#5-feature-3--persistent-websocket-tool-servers)
6. [Feature 4 — Agentic Model Chaining & Pipeline Orchestration](#6-feature-4--agentic-model-chaining--pipeline-orchestration)
7. [Feature 5 — Background Task Scheduler](#7-feature-5--background-task-scheduler)
8. [Feature 6 — System & Memory Management Tools](#8-feature-6--system--memory-management-tools)
9. [Feature 7 — Multi-Protocol API Compatibility](#9-feature-7--multi-protocol-api-compatibility)
10. [Feature 8 — Cloud Proxy & Remote Model System](#10-feature-8--cloud-proxy--remote-model-system)
11. [Feature 9 — Bearer Token Security](#11-feature-9--bearer-token-security)
12. [Feature 10 — Binary Memory Persistence](#12-feature-10--binary-memory-persistence)
13. [The Cumulative Impact: Before vs. After](#13-the-cumulative-impact-before-vs-after)
14. [Conclusion](#14-conclusion)

---

## 1. Executive Summary

Loom began its life as a clean, minimal LLM inference router — a Go HTTP server that loaded GGUF model weights, forwarded prompts into a `llama-server` subprocess, and streamed tokens back to the client. Elegant in simplicity, but fundamentally limited in ambition.

Over the course of this engineering sprint, we transformed Loom into something far more powerful: a **stateful, memory-persistent, tool-aware, multi-model orchestration layer** capable of autonomous operation. This is not a surface-level feature addition. Every change went deep into the core request pipeline — `routes.go`, `memory_middleware.go`, `model_chain.go` — rearchitecting the fundamental prompt assembly and inference dispatch loops.

This document is the complete technical record of that journey.

---

## 2. Architecture Overview & Data Flow

### The Old Architecture (Before)

```
Client Request
      │
      ▼
  HTTP Router (routes.go)
      │
      ▼
  Prompt Assembly
  [ UserMessage + SystemPrompt + ALL_TOOLS_JSON ]
      │                     ▲
      │          (hardcoded injection of
      │           ALL memory tools +
      │           ALL chain tools +
      │           ALL custom tools)
      ▼
  llama.cpp subprocess
      │
      ▼
  Stream tokens → Client
```

**Problems:** Context bloat, lost state on restart, no scheduling, no delegation.

---

### The New Architecture (After)

```
Client Request
      │
      ▼
  HTTP Router (routes.go)
      │
      ├──► tokenAuthMiddleware()  [Bearer Token Check]
      │
      ├──► Cloud Proxy Check      [RemoteHost/RemoteModel → Forward & return]
      │
      ├──► Memory Engine          [Vector search → inject top-K relevant memories]
      │
      ├──► Tool Manager Check     [If tools connected → inject toolmanager.search ONLY]
      │
      ├──► Model Fallback         [No model in req? → use default_model from config]
      │
      ▼
  Lean Prompt Assembly
  [ SystemPrompt + Memories + UserMessage + toolmanager.search ]
      │
      ▼
  llama.cpp subprocess
      │
      ├──► Model calls toolmanager.search(query)
      │         │
      │         ▼
      │    ToolManager.SearchTools(query)  [kNN cosine similarity on LanceDB]
      │         │
      │         ▼
      │    Returns matching tool schema (save_memory / chain_request / custom)
      │         │
      │         ▼
      │    Model re-invokes with actual tool
      │         │
      │         ├──► IsMemoryTool?  → ExecuteMemoryTool() → LanceDB
      │         ├──► IsChainTool?   → ExecuteChainPipeline() → specialist model
      │         ├──► IsScheduleTool?→ TaskScheduler.AddJob() → background
      │         └──► IsWebSocketTool?→ WebSocket tunnel → external Python script
      │
      ▼
  Stream tokens → Client
      │
      ▼
  Background: collectAndStoreMemories()
  [Async fact extraction → LanceDB long-term memory store]
```

---

## 3. Feature 1 — Native Long-Term Memory System

### The Problem

Every LLM inference is, by design, a stateless computation. Pass the same input twice, get the same output. This is mathematically clean but practically useless for personal AI assistants that must recall your name, your active project, your coding preferences, and your previous error messages across sessions separated by hours or days.

The naive workaround — appending a raw `CHAT_HISTORY` dump to every prompt — is quadratic in cost. A 10,000-token conversation history injected into every single new request burns through context budget and obliterates TTFT (time to first token) performance.

### The "What"

We built a **hierarchical, vector-indexed, persistent memory engine** directly into the Loom backend. Memories are:
- Extracted autonomously from every conversation response via pattern matching.
- Stored as embedding vectors in a **LanceDB** database at `~/.loom/memory.db`.
- Cached in-memory via a **Ristretto** TTL cache for sub-millisecond hot-path reads.
- Ranked by a decay-weighted importance score and injected into the system prompt as a concise context block on every subsequent request.

### The "How" (Implementation Deep Dive)

**Initialization** (`server/memory_middleware.go: initMemoryEngine`):
```
Server boot
    │
    ├── memory.LoadConfig()  → reads ~/.loom/server.json
    │       ├── enabled: true/false
    │       ├── db_path: ~/.loom/memory.db (LanceDB)
    │       ├── embedding_model: (local Loom model name)
    │       ├── top_k: (max memories injected per request)
    │       ├── similarity_threshold: (cosine cutoff)
    │       └── importance_threshold: (decay-weighted score cutoff)
    │
    └── memory.NewEngine(cfg)
            ├── store_lancedb.New()     → opens/creates LanceDB tables
            ├── embedder_loom.New()   → connects to local embedding model
            ├── cache_ristretto.New()   → initializes in-memory LRU cache
            └── eng.Start(ctx)          → launches background decay processor goroutine
```

**On Every Chat Request** (`injectMemoryIntoMessages`):
```
Incoming /api/chat
    │
    ├── Extract userID from peer address
    │
    ├── Engine.ProcessRequest(userID, messages)
    │       ├── Embed last user message → float32 vector
    │       ├── BruteForceIndex.Search(query_vector, top_k)
    │       │       └── cosine_similarity(v_i, query) for all stored memories
    │       │           → sorted by (similarity × importance_score)
    │       └── prompt_builder.Build(relevant_memories)
    │               → "## Relevant Context\n- User's name is Tuhin\n- Prefers Python..."
    │
    └── Prepend enriched system prompt to messages
```

**Post-Inference** (`collectAndStoreMemories` → `storeResponseMemories`):
```
Streaming response channel
    │
    ├── Wrap channel: accumulate tokens without blocking stream
    │
    └── On stream close → background goroutine:
            ├── extractor_pattern.Extract(full_response)
            │       → Regex + heuristic extraction of facts
            │           e.g. "My name is Tuhin" → {content: "User name is Tuhin", type: "user"}
            ├── embedder.Embed(extracted_fact)
            │       → float32 vector via local embedding model
            └── store.Upsert(memory_record)
                    └── LanceDB insert with metadata:
                            {id, userID, type, content, embedding, importance, timestamp}
```

### Memory Categories

| Type | Description | Example |
|---|---|---|
| `user` | Persistent personal facts | "User's name is Tuhin" |
| `project` | Active project context | "Working on Loom tool architecture" |
| `conversation` | Session-scoped summaries | "Discussed RAG implementation" |
| `episodic` | Time-stamped events | "Deployed at 2026-07-29" |
| `semantic` | Conceptual knowledge | "Prefers Go over Python for backends" |

### Problem Solved

| Before | After |
|---|---|
| Context resets every session | Persistent cross-session memory |
| Full history dump → context bloat | Top-K vector search → lean injection |
| O(N) context cost grows with history | O(1) cost: always top_k memories |
| No fact extraction | Autonomous background fact extraction |

---

## 4. Feature 2 — RAG Tool Manager

### The Problem

Function-calling in LLMs requires the orchestrator to embed complete JSON schemas for every available tool inside the system prompt. With 11 built-in Loom tools and potentially hundreds of external developer tools connected via WebSocket, this approach is catastrophically inefficient:

- 11 built-in tools × ~200 tokens each = **~2,200 tokens consumed on every single request**
- External developer tools on top = easily **4,000–6,000 tokens wasted pre-fill**
- Anthropic's research demonstrates attention quality degrades linearly with irrelevant context ("Lost in the Middle" phenomenon)

### The "What"

We implemented a **RAG-based Tool Discovery System** backed by a volatile **LanceDB** vector index at `~/.loom/toolsmanager_db`. The entire taxonomy of available tools — both Loom's 11 built-ins and any externally connected tools — is stored as high-dimensional embedding vectors. The LLM is given a single, ultra-concise **meta-tool** called `toolmanager.search` in its context window. Tool retrieval is demand-driven, not pre-loaded.

### The "How" (Implementation Deep Dive)

**Server Initialization** (`server/tool_manager.go: InitToolManager`):
```
./loom serve
    │
    ├── os.RemoveAll(~/.loom/toolsmanager_db)   ← WIPE stale data
    │
    ├── lancedb.Connect(toolsmanager_db)           ← Create fresh DB
    │
    ├── globalToolManager = &ToolManager{
    │       store: lancedb_table,
    │       index: memory.NewBruteForceIndex(),    ← In-memory cosine index
    │       dbDir: dbDir,
    │   }
    │
    └── globalToolManager.RegisterBuiltinTools(ctx, s)
            ├── GetMemoryTools()     → 6 memory tools
            └── GetChainTools()      → 2 chain/schedule tools
                │
                └── For each tool:
                        ├── embedder.Embed(tool.Function.Description)
                        └── BruteForceIndex.Add(embedding, tool_schema)
```

**The Meta-Tool Injection** (`server/routes.go: ChatHandler`):
```
Incoming /api/chat
    │
    ├── globalToolServer.GetActiveTools() → check connected WebSocket tools
    │
    └── If active_tools > 0:
            ├── json.Unmarshal(toolmanager_search_schema) → api.Tool
            │       {
            │           name: "toolmanager.search",
            │           description: "Search the vector database for a tool...",
            │           parameters: { query: string }
            │       }
            ├── req.Tools = append(req.Tools, searchTool)
            └── Inject system note:
                    "[System: Use toolmanager.search to find tools dynamically]"
```

**Runtime Tool Retrieval** (when model calls `toolmanager.search`):
```
Model generates: tool_call { name: "toolmanager.search", args: { query: "save a memory" } }
    │
    ├── ToolManager.SearchTools("save a memory", k=3)
    │       ├── embedder.Embed("save a memory")     → query_vector
    │       ├── BruteForceIndex.Search(query_vector, k)
    │       │       └── for v_i in index:
    │       │               score_i = dot(v_i, query_vector)  [cosine, L2-normalized]
    │       │           → top-k results sorted by score
    │       └── Returns: [save_memory_schema, list_memories_schema, ...]
    │
    └── Inject returned schema into conversation
            → Model now calls save_memory(...) with full knowledge of parameters
```

### Architecture Diagram: Tool Retrieval Flow

```
┌─────────────────────────────────────────────────────────┐
│                  LOOM CONTEXT WINDOW                  │
│                                                         │
│  [System Prompt]  [Memories]  [Chat History]            │
│                                                         │
│  Tools: [ toolmanager.search ]   ← ONLY 1 tool         │
│         (Cost: ~50 tokens)       ← vs 2,200+ before    │
└───────────────────┬─────────────────────────────────────┘
                    │  Model calls toolmanager.search(query)
                    ▼
          ┌─────────────────────┐
          │   ToolManager       │
          │   (LanceDB + kNN)   │
          │                     │
          │  toolsmanager_db/   │
          │  ├── save_memory    │
          │  ├── list_memories  │
          │  ├── delete_memory  │
          │  ├── save_special   │
          │  ├── chain_request  │
          │  ├── schedule_task  │
          │  ├── read_sys_logs  │
          │  ├── system_tool    │
          │  └── [custom tools] │
          └─────────┬───────────┘
                    │  Returns matching schema
                    ▼
          Model now has the exact schema it needs
          → Calls the actual tool
```

### Problem Solved

| Metric | Before | After |
|---|---|---|
| Tokens per request (tools) | ~2,200–6,000 | ~50 (1 meta-tool) |
| Max supported tools | ~20 (context limit) | Unlimited |
| Context for user content | Severely limited | Maximized |
| Stale disconnected tools | Ghost schemas in prompt | Wiped on restart |
| Hallucination risk | High (irrelevant schemas confuse model) | Minimal |

---

## 5. Feature 3 — Persistent WebSocket Tool Servers

### The Problem

Integrating custom tools (Python scripts, database connectors, browser automation) with a local LLM has historically required external orchestration frameworks (LangChain, AutoGen, LlamaIndex). These frameworks intercept LLM output via polling, run tool logic externally, then re-inject results via a new HTTP POST. This pattern introduces serialization overhead, breaks native streaming, and requires the client application to own the execution loop.

### The "What"

We built a native `ToolInterfaceHandler` at `/api/tools/interface` within the Loom HTTP router. External processes establish persistent WebSocket connections, register their tool schemas, and receive execution dispatch events directly from the inference loop. This eliminates all polling overhead and integrates tool execution as a first-class streaming primitive.

### The "How" (Implementation Deep Dive)

**Protocol Flow** (`server/tool_interface.go`):
```
External Python Script                    Loom Server
       │                                       │
       ├──── WS Connect to /api/tools/interface ──►│
       │                                       │
       ├──── AuthMessage { token: "..." } ────►│ tokenAuthMiddleware
       │                                       │
       ├──── RegisterMessage {                 │
       │       tools: [                        │
       │         { name: "run_python",         │
       │           description: "Execute...",  │
       │           parameters: { code: str }   │
       │         }                             │
       │       ]                               │
       │     } ──────────────────────────────►│ globalToolManager.AddTool()
       │                                       │   → embed description
       │                                       │   → insert into LanceDB index
       │                                       │
       │  ... user sends chat message ...      │
       │                                       │
       │ ◄── ExecuteMessage {                  │
       │       tool_call_id: "tc_01",          │
       │       name: "run_python",             │
       │       arguments: { code: "print(42)"}│
       │     } ───────────────────────────────│ Inference loop suspends
       │                                       │
       ├──── ResultMessage {                   │
       │       tool_call_id: "tc_01",          │
       │       result: "42"                    │
       │     } ──────────────────────────────►│ Inject result into context
       │                                       │ Resume inference stream
       │                                       │
       ├──── WS Disconnect ──────────────────►│ globalToolManager.RemoveTool()
                                               │   → delete from LanceDB index
```

**WebSocket Lifecycle & Tool Sync** (`server/tool_interface.go`):
```go
// On connect:
globalToolServer.RegisterClient(conn, registeredTools)

// On tool registration:
for _, tool := range tools {
    globalToolManager.AddTool(ctx, tool.Function.Name, tool)
    // → Embed description
    // → Insert into BruteForceIndex
    // → Insert schema into LanceDB table
}

// On disconnect:
for _, toolName := range client.ToolNames {
    globalToolManager.RemoveTool(ctx, toolName)
    // → Delete from BruteForceIndex
    // → Delete from LanceDB table
}
```

### Problem Solved

| Before | After |
|---|---|
| External Python runtime owns the execution loop | Loom natively owns the execution loop |
| Polling-based tool call detection | Event-driven WebSocket dispatch |
| Tool execution breaks streaming | Streaming transparent to tool calls |
| Tool definitions injected in every HTTP request payload | Tools registered once, retrieved via RAG |
| No dynamic tool registration | Connect/disconnect dynamically |

---

## 6. Feature 4 — Agentic Model Chaining & Pipeline Orchestration

### The Problem

Local inference is constrained by VRAM. A Qwen3-8B model that handles general conversation cannot simultaneously hold a LLaVA vision model in GPU memory. Furthermore, there is no universally optimal model for all tasks — code generation, mathematical reasoning, multimodal vision, and embedding generation each have specialized architectures.

### The "What"

We implemented a **multi-stage pipeline orchestrator** (`server/model_chain.go`) that enables a primary LLM to decompose a request into typed subtasks and delegate each subtask to a dynamically selected specialist model. The orchestrator manages the full VRAM lifecycle: unloading the primary model, sequentially loading and executing specialists, passing context between steps, reloading the primary model, and returning aggregated results.

### The "How" (Implementation Deep Dive)

**Model Capability Resolution** (`selectModelForTask`):
```
chain_request received with sub_task { required_capability: "vision" }
    │
    └── selectModelForTask(ctx, "vision", preferred_model)
            ├── List all local models via model_list_cache
            ├── For each model:
            │       score = 0
            │       if model.Name contains "vision" or "vl" or "llava" → score++
            │       if model.Description contains "vision" → score++
            │       if model.SystemPrompt contains "vision" → score++
            │       if model.Capabilities has CapabilityVision → score++
            └── Return highest-scoring model
```

**Pipeline Execution Flow** (`ExecuteChainPipeline`):
```
User: "Analyze this chart image and write a Python script for the data"
    │
    ▼
Primary Model (e.g. Qwen3-8B):
    Calls chain_request {
        reason: "Need vision for image + code for script",
        sub_tasks: [
            { required_capability: "vision",
              prompt: "Describe the data in this chart image",
              needs_previous_output: false },
            { required_capability: "code",
              prompt: "Write Python to visualize this data: {prev_output}",
              needs_previous_output: true }
        ]
    }
    │
    ▼
ExecuteChainPipeline():
    │
    ├── Step 1: Vision
    │       ├── selectModelForTask("vision") → "llava:7b"
    │       ├── Unload Qwen3-8B from VRAM
    │       ├── Load llava:7b
    │       ├── Execute prompt with image context
    │       ├── Capture output: "The chart shows revenue growth of 42% in Q3..."
    │       └── Stream progress: "[Step 1/2: Vision ✓]"
    │
    ├── Step 2: Code
    │       ├── selectModelForTask("code") → "qwen2.5-coder:7b"
    │       ├── Unload llava:7b from VRAM
    │       ├── Load qwen2.5-coder:7b
    │       ├── Inject previous output as context
    │       ├── Execute: "Write Python for: The chart shows revenue growth..."
    │       ├── Capture output: "import matplotlib\n..."
    │       └── Stream progress: "[Step 2/2: Code ✓]"
    │
    └── Reload Qwen3-8B
        Synthesize: "Here is the analysis and the script: [...]"
        Stream final response to user
```

**Modelfile DESCRIPTION Integration:**
```
# ~/.loom/models/manifests/.../llava:7b (Modelfile)
FROM llava:7b
DESCRIPTION A multimodal vision-language model supporting image analysis and OCR.
SYSTEM You are a vision AI assistant.
```
The `DESCRIPTION` field is parsed by `server/create.go`, stored in `model.ConfigV2`, cached in `model_list_cache.go`, and surfaced to `selectModelForTask()` for semantic routing.

### Problem Solved

| Before | After |
|---|---|
| Single model per request | Multi-model pipeline orchestration |
| Vision impossible on text-only models | Transparent vision delegation |
| User must manually switch models | Fully automatic capability routing |
| No context passing between models | Step output passed as next step input |
| VRAM overflow crashes | Managed load/unload lifecycle |

---

## 7. Feature 5 — Background Task Scheduler

### The Problem

LLMs executing user requests are synchronous by necessity — the user is waiting for a response. But many useful AI operations are inherently asynchronous: "Summarize my emails every morning at 9 AM," "Remind me to review this PR in 2 hours," "Run this data extraction pipeline at midnight." Achieving this traditionally requires a separate cron infrastructure entirely external to the AI runtime.

### The "What"

We built a native **in-process background job scheduler** (`server/task_scheduler.go`) that persists jobs to `~/.loom/scheduled_jobs.json`. The scheduler supports one-shot execution at RFC3339 timestamps, relative delay offsets (`"in 5m"`, `"2h30m"`), and full 5-field cron expressions (`*/5 * * * *`).

### The "How" (Implementation Deep Dive)

**Architecture:**
```
┌─────────────────────────────────────┐
│         Task Scheduler              │
│                                     │
│  tick() goroutine  (every 30s)      │
│       │                             │
│       ├── Load jobs from disk       │
│       ├── For each job:             │
│       │    if nextRun <= now:       │
│       │       executeJob(job)       │
│       │       │                     │
│       │       ├── HTTP POST         │
│       │       │   /api/chat         │
│       │       │   { model, prompt } │
│       │       │         │           │
│       │       │         ▼           │
│       │       │   Inference Loop    │
│       │       │         │           │
│       │       │         ▼           │
│       │       └── Store result      │
│       │                             │
│       └── For cron jobs:            │
│            nextRun = cron.Next(now) │
│            Save updated state       │
└─────────────────────────────────────┘
```

**ParseScheduleTime — Flexible Time Parser:**
```go
// Supported formats:
"in 5m"          → time.Now().Add(5 * time.Minute)
"2h30m"          → time.Now().Add(2.5 * time.Hour)
"2026-07-29T..."  → time.Parse(time.RFC3339, s)
"*/5 * * * *"    → cron.Next(time.Now())
```

**LLM Interface via `schedule_task` tool:**
```json
{
  "action": "schedule",
  "prompt": "Summarize the last hour of logs",
  "run_at": "in 1h",
  "model": "qwen3:8b"
}
```
```json
{
  "action": "schedule",
  "prompt": "Generate daily standup report",
  "cron": "0 9 * * 1-5"
}
```

**Persistence Schema** (`~/.loom/scheduled_jobs.json`):
```json
[
  {
    "id": "job_abc123",
    "prompt": "...",
    "model": "...",
    "run_at": "2026-07-30T09:00:00Z",
    "cron": "",
    "status": "pending",
    "result": "",
    "created_at": "2026-07-29T..."
  }
]
```

### Problem Solved

| Before | After |
|---|---|
| No scheduling capability | Full cron + one-shot scheduler |
| Required external cron + API call | Native in-process goroutine |
| No job persistence | JSON persistence across restarts |
| No result retrieval | Stored results queryable via LLM |

---

## 8. Feature 6 — System & Memory Management Tools

### The Problem

A truly autonomous AI agent must be able to introspect and manipulate its own runtime environment — reading system logs to diagnose errors, loading or unloading specific models to manage VRAM, and saving critical facts for future reference. Without first-class tooling for these operations, the AI remains a passive responder rather than an active operator.

### The "What"

We expose **11 native function-calling tools** to LLMs via the `GetMemoryTools()` function, covering four domains: memory CRUD operations, special/pinned memory management, system diagnostics, and runtime control.

### Tool Taxonomy

**Memory Operations:**
```
save_memory(content, type, tags)
    → Creates new memory record in LanceDB
    → type: user | project | conversation | episodic | semantic

list_memories(type?, pinned?, archived?)
    → Queries LanceDB with optional filters
    → Returns formatted memory list with IDs

delete_memory(id)
    → Removes memory record by UUID from LanceDB + BruteForceIndex
```

**Special (Pinned) Memory — High-Importance Key-Value Store:**
```
save_special_memory(key, value)
    → High-importance persistent fact (e.g. "API_KEY", "CURRENT_PROJECT")

list_special_memories()
    → Returns all pinned special memories

delete_special_memory(key)
    → Removes special memory by key
```

**System Observability:**
```
read_system_logs(lines?)
    → Tails server stderr log
    → Returns last N lines of runtime log output

check_data_flow()
    → Diagnostic snapshot: memory engine health, connected tools count,
      scheduler job count, active models, VRAM usage

restart_server()
    → Executes graceful server restart via SIGTERM + re-exec
```

**Runtime Control:**
```
system_tool(action, params)
    → action: "load_model"    → forces model into VRAM
    → action: "unload_model"  → evicts model from VRAM
    → action: "get_status"    → returns server health JSON
    → action: "list_models"   → returns local model registry
    → action: "restart"       → triggers graceful restart
```

### Problem Solved

| Capability | Before | After |
|---|---|---|
| AI reads its own logs | Impossible | `read_system_logs` |
| AI manages VRAM models | Impossible | `system_tool(load/unload)` |
| AI saves user facts | Impossible | `save_memory` |
| AI pins critical data | Impossible | `save_special_memory` |
| AI checks its own health | Impossible | `check_data_flow` |

---

## 9. Feature 7 — Multi-Protocol API Compatibility

### The Problem

The LLM API ecosystem has fragmented into at least three competing protocol standards: Loom native (`/api/chat`), OpenAI v1 (`/v1/chat/completions`), and Anthropic (`/v1/messages`). Client applications built for OpenAI cannot directly consume Loom's API without modification — forcing developers to choose between ecosystem compatibility and local inference.

### The "What"

Loom's `routes.go` exposes a unified Gin HTTP router serving all three protocol families from a single process, with full request/response translation layers.

### Protocol Coverage

**Native Loom API:**
```
POST   /api/generate          → GenerateHandler
POST   /api/chat              → ChatHandler
POST   /api/embed             → EmbedHandler
GET    /api/tags              → ListHandler
POST   /api/show              → ShowHandler
POST   /api/create            → CreateHandler
DELETE /api/delete            → DeleteHandler
POST   /api/copy              → CopyHandler
POST   /api/pull              → PullHandler
POST   /api/push              → PushHandler
GET    /api/ps                → PsHandler
GET    /api/status            → StatusHandler
```

**OpenAI v1 Compatibility:**
```
POST   /v1/chat/completions   → OpenAI Chat (stream + non-stream)
POST   /v1/completions        → Legacy text completion
POST   /v1/embeddings         → Embedding generation
GET    /v1/models             → Model list (OpenAI format)
GET    /v1/models/:model      → Model detail
POST   /v1/responses          → Response API
POST   /v1/images/generations → Image generation (delegated via chain)
POST   /v1/audio/transcriptions→ Audio transcription (delegated via chain)
```

**Anthropic v1 Translation:**
```
POST   /v1/messages           → AnthropicMessagesHandler
    ├── Translates Anthropic message format → Loom native format
    ├── Handles content blocks (text, image, tool_use, tool_result)
    ├── Dynamic vision model routing via selectModelForTask()
    └── Translates Loom response → Anthropic response format
```

**Experimental APIs:**
```
GET    /api/experimental/web_search           → WebSearchExperimentalHandler
GET    /api/experimental/web_fetch            → WebFetchExperimentalHandler
GET    /api/experimental/model-recommendations→ ModelRecommendationsExperimentalHandler
```

### The Anthropic Vision Routing Detail

The `AnthropicMessagesHandler` has a particularly elegant sub-feature: when a request arrives with an image payload and an empty or generic model name (e.g. `"claude"`), it transparently invokes `selectModelForTask(ctx, "vision", "")` to dynamically resolve an appropriate local vision model, eliminating the need for clients to hardcode local model identifiers.

### Problem Solved

| Client Type | Before | After |
|---|---|---|
| OpenAI SDK clients | Must rewrite API calls | Zero-code compatibility |
| Anthropic SDK clients | Must rewrite API calls | Transparent translation |
| Vision clients | Must specify model name | Auto-resolved vision model |
| Streaming clients | Protocol-dependent | Unified SSE streaming |

---

## 10. Feature 8 — Cloud Proxy & Remote Model System

### The Problem

Enterprises and developers frequently need access to both local quantized models (for privacy, cost, and latency) and cloud-hosted models (for maximum capability on complex tasks) within the same application. Maintaining two separate API clients and branching logic on the application side adds significant complexity and maintenance burden.

### The "What"

Loom's `cloud_proxy.go` implements a transparent **reverse proxy layer** that intercepts inference requests targeting models configured with `RemoteHost` and `RemoteModel` properties in their Modelfile, forwarding them to the specified remote endpoint while presenting a unified local API surface to the client.

### The "How"

**Modelfile Configuration:**
```
FROM remote
PARAMETER remote_host https://api.openai.com
PARAMETER remote_model gpt-4o
```

**Request Interception Flow:**
```
POST /api/chat { model: "gpt4-proxy" }
    │
    ├── Load model config → detect RemoteHost, RemoteModel
    │
    ├── cloudPassthroughMiddleware()
    │       ├── Translate Loom request → target protocol format
    │       ├── Forward to https://api.openai.com/v1/chat/completions
    │       ├── Stream remote response back to local client
    │       └── Handle auth headers (Bearer token passthrough)
    │
    └── Client receives response as if from local model
```

**Key Functions:**
- `proxyCloudJSONRequest()`: Core reverse-proxy implementation
- `cloudPassthroughMiddleware()`: Route-level interception for model endpoints
- `cloudModelPathPassthroughMiddleware()`: Path-based interception for `/v1/models/:model`

### Problem Solved

| Before | After |
|---|---|
| Two separate API clients required | Single unified Loom API |
| Manual model routing logic in application | Transparent proxy at model config level |
| Hardcoded cloud API endpoints | Configurable per-model via Modelfile |

---

## 11. Feature 9 — Bearer Token Security

### The Problem

Loom serves its API on `0.0.0.0` by default in many deployment configurations. Without authentication, any process on the host or network can invoke `/api/chat`, potentially abusing local compute resources or exfiltrating context from sensitive conversations.

### The "What"

We implemented a **Bearer token authentication middleware** (`tokenAuthMiddleware`) that enforces an `Authorization: Bearer <token>` header on all API endpoints, with the token configured via `~/.loom/server.json`.

### The "How"

**Configuration:**
```json
// ~/.loom/server.json
{
  "api_token": "your-secret-token-here"
}
```

**Middleware Implementation:**
```
Incoming HTTP Request
    │
    ├── Path == "/" (health check) → Allow (no auth required)
    │
    └── Extract "Authorization" header
            ├── Missing header → 401 Unauthorized
            ├── Not "Bearer ..." format → 401 Unauthorized
            ├── Token != config.api_token → 401 Unauthorized
            └── Token matches → c.Next() [proceed to handler]
```

**Exempt Paths:**
- `/` (health check) — always public, enables load balancer probes without credentials

### Problem Solved

| Before | After |
|---|---|
| Fully open API | Token-authenticated API |
| Any LAN host can invoke inference | Only authorized clients |
| No protection on sensitive memory endpoints | All endpoints protected |
| No central auth config | Single config entry in server.json |

---

## 12. Feature 10 — Binary Memory Persistence

### The Problem

Loom's memory export system originally serialized all memory records, conversations, and special memories to JSON. While human-readable, JSON serialization for large memory databases suffers from:
- Verbose token overhead (field names repeated for every record)
- CPU overhead in `encoding/json` marshaling/unmarshaling for large record sets
- Fragile format for binary data (embeddings must be base64-encoded)

### The "What"

We re-engineered the default memory export/import format from JSON to Go's native **binary `encoding/gob`** format, retaining JSON as an explicitly opt-in alternative via `--format json`.

### The "How"

**Export Flow** (`cmd/cmd.go`):
```go
// Default: binary gob
encoder := gob.NewEncoder(file)
encoder.Encode(exportData)  // compact, efficient binary

// Opt-in: human-readable JSON
if format == "json" {
    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    encoder.Encode(exportData)
}
```

**Import Flow:**
```go
// Auto-detect format by extension
if strings.HasSuffix(file, ".dat") {
    gob.NewDecoder(f).Decode(&data)
} else if strings.HasSuffix(file, ".json") {
    json.NewDecoder(f).Decode(&data)
}
```

### Problem Solved

| Metric | JSON (.json) | Binary Gob (.dat) |
|---|---|---|
| Serialization speed | Slow (string parsing) | Fast (native binary) |
| File size | Large (verbose field names) | Compact |
| Embedding storage | Base64 overhead | Native float32 arrays |
| Default format | Was default | Now default |

---

## 13. The Cumulative Impact: Before vs. After

### Token Budget Per Request

```
Before (Traditional Injection):
┌──────────────────────────────────────────┐  8,192 tokens total
│ System Prompt         │ ~200 tokens      │
│ Memory Tools (×6)     │ ~1,200 tokens    │
│ Chain Tools (×2)      │ ~400 tokens      │
│ Chain Instructions    │ ~300 tokens      │
│ Custom Tools (×10)    │ ~2,000 tokens    │  ← Wasted: 4,100 tokens
│ Chat History          │ ~2,000 tokens    │
│ User Message          │ ~92 tokens       │
└──────────────────────────────────────────┘

After (RAG Tool Architecture):
┌──────────────────────────────────────────┐  8,192 tokens total
│ System Prompt         │ ~200 tokens      │
│ toolmanager.search    │ ~50 tokens       │  ← Only 50 tokens overhead!
│ System Note (1 line)  │ ~30 tokens       │
│ Relevant Memories     │ ~300 tokens      │
│ Chat History          │ ~5,000 tokens    │  ← 5x more conversation space
│ User Message          │ ~2,612 tokens    │  ← Room for large inputs
└──────────────────────────────────────────┘
```

### Feature Impact Matrix

```
┌─────────────────────────────┬────────────┬────────────────────────────────┐
│ Feature                     │ Status     │ Core Benefit                   │
├─────────────────────────────┼────────────┼────────────────────────────────┤
│ Long-Term Memory            │ ✅ ACTIVE  │ Cross-session state persistence │
│ RAG Tool Manager            │ ✅ ACTIVE  │ 98% context window savings      │
│ WebSocket Tool Servers      │ ✅ ACTIVE  │ Native external tool execution  │
│ Model Chaining              │ ✅ ACTIVE  │ Multi-model pipeline delegation │
│ Task Scheduler              │ ✅ ACTIVE  │ Autonomous background execution │
│ System Management Tools     │ ✅ ACTIVE  │ AI self-introspection & control │
│ Multi-Protocol API          │ ✅ ACTIVE  │ OpenAI + Anthropic compatibility│
│ Cloud Proxy                 │ ✅ ACTIVE  │ Local + cloud unified API       │
│ Bearer Token Security       │ ✅ ACTIVE  │ Protected LAN deployments       │
│ Binary Memory Persistence   │ ✅ ACTIVE  │ Fast, compact memory I/O        │
└─────────────────────────────┴────────────┴────────────────────────────────┘
```

---

## 14. Conclusion

What started as a stateless HTTP-to-llama.cpp proxy has been transformed into a comprehensive AI operating layer. The architectural choices made throughout this engineering sprint were deliberately conservative in scope but radical in impact:

- We did not add external dependencies arbitrarily — LanceDB was already part of the memory system; we simply extended its role to cover tools.
- We did not break backward compatibility — all existing Loom API clients continue to work without modification.
- We did not compromise on performance — every new feature was designed to *reduce* computational overhead on the hot inference path, not increase it.

The result is an Loom that is simultaneously more capable, more efficient, and more extensible than its predecessor. Local AI is no longer constrained to stateless, single-turn inference. It is now a fully stateful, multi-model, self-scheduling autonomous system — all running on your own hardware.

---

## Appendix: Deep Technical Specifics

### Task Scheduler: Internal Tick Architecture

```
initTaskScheduler()
    │
    └── globalScheduler = &TaskScheduler{
            jobs:     load(~/.loom/scheduler.json),  ← Heal "running" → "failed"
            ticker:   time.NewTicker(15 * time.Second), ← 15s resolution
            doneJobRetention: 24 * time.Hour,
        }
        │
        └── go run(ctx)
                │
                └── tick(ctx) on every interval:
                        ├── Scan jobs where status == "pending" && RunAt <= now
                        ├── Set status = "running" → save state
                        ├── go executeJob(job) → runPrompt(model, system, user)
                        │       └── r.Completion() → llama.cpp inference
                        ├── For cron jobs: nextRun = nextCronTime(expr, now)
                        └── Prune done one-shot jobs older than 24h
```

**Cron Parser Capabilities:**
| Expression | Meaning |
|---|---|
| `*/5 * * * *` | Every 5 minutes |
| `0 9 * * 1-5` | 9 AM weekdays |
| `30 23 1,15 * *` | 11:30 PM on 1st and 15th |
| `0 */2 * * *` | Every 2 hours |

---

### Memory Engine: Full Hot-Path Enrichment Pipeline

```
ProcessRequest(userID, messages):
    │
    ├── 1. Check Ristretto Cache (TTL-based LRU)
    │         Hit  → Return cached memories (sub-millisecond)
    │         Miss → Proceed to vector search
    │
    ├── 2. Embed last user message
    │         embedder.Embed(lastUserMessage) → float32 vector (e.g. 1024-dim)
    │
    ├── 3. BruteForceIndex.Search(query_vector, top_k)
    │         for each indexed memory vector v_i:
    │             score_i = dot(L2_normalize(v_i), L2_normalize(query_vector))
    │         sort by score DESC → take top_k
    │
    ├── 4. Filter by similarity_threshold
    │         Remove memories where score < threshold
    │
    ├── 5. LanceDB.GetByIDs(filtered_ids)
    │         Fetch full memory records including metadata
    │
    ├── 6. Ranker.Score(memories)
    │         final_score = similarity × importance_score × recency_weight
    │         Pinned memories receive priority boost
    │
    ├── 7. Apply max_prompt_tokens budget
    │         Greedily add memories until token budget exceeded
    │
    ├── 8. PromptBuilder.Build(ranked_memories)
    │         "## Relevant Context from Memory\n"
    │         "- User's name is Tuhin (importance: 0.80)\n"
    │         "- Working on Loom RAG architecture (importance: 0.75)\n"
    │
    └── 9. Prepend to system prompt → forward to llama.cpp
```

---

### System Tool: VRAM Lifecycle via Scheduler Runner

```go
// load_model action:
s.sched.getRunner(ctx, modelRef, sessionDuration, nil)
    → Allocates GPU memory layers
    → Starts llama-server subprocess
    → Returns runner handle

// unload_model action:
s.sched.expireRunner(modelName)
    → Sets TTL = 0 on runner
    → llama-server process receives SIGTERM
    → GPU memory deallocated

// get_status action:
runners := s.sched.loaded          // active runner map
warnCount = countLogPattern("WARN")
errCount  = countLogPattern("ERR")
```

---

### Special Memory: Vector-Indexed Key-Value Store

Unlike standard episodic memories which are free-form text, **Special Memories** function as a structured, semantically searchable key-value store:

```
save_special_memory(key="CURRENT_PROJECT", value="Loom RAG Architecture")
    │
    ├── content = "CURRENT_PROJECT: Loom RAG Architecture"
    ├── embedding = embedder.Embed(content)
    ├── SpecialMemory{
    │       ID:        uuid.New(),
    │       UserID:    userID,
    │       Key:       "CURRENT_PROJECT",
    │       Value:     "Loom RAG Architecture",
    │       Embedding: embedding,
    │       CreatedAt: time.Now(),
    │   }
    └── store.UpsertSpecial(record)  ← replaces existing entry with same Key
```

Special memories appear prominently in the prompt builder output, giving the model high-priority persistent context about its operational environment.

---

### Model Chain: VRAM Swap Protocol

```
ExecuteChainPipeline — VRAM Management Detail:

┌────────────────────────────────────────────────┐
│ STEP 0: Assess VRAM                            │
│   if primaryModel in memory:                   │
│     unloadChainModelAndWait(primaryModel)       │
│       → poll GET /api/ps every 500ms           │
│       → timeout after 30 seconds               │
│   GPU now free                                  │
└────────────────────────────────────────────────┘
         │ for each subtask:
         ▼
┌────────────────────────────────────────────────┐
│ STEP N: Execute Specialist                     │
│   selectModelForTask(capability, keyword)       │
│   → POST /api/chat { model: specialist }       │
│   → llama.cpp loads specialist weights         │
│   → Run inference, collect output              │
│   → unloadChainModelAndWait(specialist)        │
│     (poll every 500ms, timeout 30s)            │
│   GPU now free for next step                    │
└────────────────────────────────────────────────┘
         │ all steps complete:
         ▼
┌────────────────────────────────────────────────┐
│ FINAL: Reload Primary Model                    │
│   POST /api/chat { model: primaryModel,        │
│     context: aggregated_results }              │
│   → Synthesize final response                  │
│   → Stream to user                             │
└────────────────────────────────────────────────┘
```

This precise 500ms polling with 30-second timeout ensures specialist models fully release GPU memory before loading the next model, preventing out-of-memory crashes on single-GPU systems with as little as 8GB VRAM.
