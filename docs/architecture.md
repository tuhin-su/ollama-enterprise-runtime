# Ollama Architecture Overview

This document outlines the high-level architecture of Ollama, with a specific focus on the newly introduced **Agentic Model Chaining** and **Dynamic Model Resolution** capabilities.

## 1. Core Model Chaining & Pipeline Orchestration

### Concept
Ollama now supports **Model Chaining** and **Pipeline Orchestration**, enabling a default general-purpose model to dynamically delegate complex subtasks (e.g., vision processing, coding, math) to specialized models available locally.

### Implementation
- **RAG Tool Manager**: Instead of injecting the `chain_request` tool into every prompt and bloating the context window, it is natively registered inside the `ToolManager` (a LanceDB volatile vector index).
- **Meta-Tool Search**: The model's context only contains a single meta-tool (`toolmanager.search`). When the model needs to chain tasks, it uses this tool to dynamically retrieve the `chain_request` tool.
- **Orchestration**: The `chain_request` tool acts as a bridge. If the primary model encounters a request requiring a capability it lacks (e.g., analyzing an image), it triggers the tool, specifying a `reason` and breaking the work into `sub_tasks`. 
- **Model Awareness**: When building the tool prompt, Ollama automatically fetches a list of locally available models, including their descriptions and capabilities. These are appended to the tool's description so the orchestrator model is fully aware of its environment and can supply a `preferred_model`.

## 1.5 RAG Tool Architecture & WebSocket Integrations

### Concept
Ollama implements a **Retrieval-Augmented Generation (RAG) Tool Architecture** to prevent context window bloat when exposing dozens of tools (like memory tools, scheduling, or external scripts) to the model.

### Implementation
- **Volatile Vector DB**: A LanceDB index is initialized in `~/.ollama/toolsmanager_db`. It is wiped on server startup to prevent stale tool schemas.
- **WebSocket Server**: External clients (like Python scripts) can connect via WebSockets. Their tools are dynamically embedded and inserted into the `ToolManager`.
- **Built-in Tools**: Ollama's native tools (Memory, Chaining) are registered directly into the `ToolManager` at startup instead of being hardcoded into the system prompt.

## 2. Dynamic Model Resolution

### Concept
To ensure tasks are routed to the most capable model without relying on hardcoded strings, Ollama employs dynamic model resolution based on model metadata.

### Implementation
- **Model Capabilities**: Models are mapped to capabilities such as `vision`, `code`, `math`, `tools`, etc.
- **`selectModelForTask` (in `server/model_chain.go`)**: This function receives a requested capability or keyword and scans all local models. It uses heuristic matching across the model's name, family, system prompt, and **description** to find the optimal specialist model.
- **Anthropic Proxy Translation**: The `AnthropicMessagesHandler` (in `server/routes.go`) utilizes `selectModelForTask` to dynamically assign a local vision model when a generic or empty model name (like "claude") is provided, eliminating the need to hardcode model versions.

## 3. Persistent Metadata: Model Descriptions

### Concept
Models need to carry semantic meaning about their purpose to aid the orchestrator in making intelligent routing decisions. 

### Implementation
- **Modelfile `DESCRIPTION`**: The `parser` natively recognizes the `DESCRIPTION` keyword in `Modelfiles`.
- **Config & Manifest Integration**: The description is captured in the API's `CreateRequest`, persisted in the model's configuration (`model.ConfigV2`), and baked into the manifest metadata upon creation (`server/create.go`).
- **Visibility**: The `modelListSummary` struct caches this description, allowing it to be surfaced efficiently via `ollama list` and `ollama show` endpoints, and crucially, fed into the `GetChainTools()` tool descriptions for agentic awareness.
