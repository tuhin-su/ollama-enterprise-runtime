# Ollama Architecture Overview

This document outlines the high-level architecture of Ollama, with a specific focus on the newly introduced **Agentic Model Chaining** and **Dynamic Model Resolution** capabilities.

## 1. Core Model Chaining & Pipeline Orchestration

### Concept
Ollama now supports **Model Chaining** and **Pipeline Orchestration**, enabling a default general-purpose model to dynamically delegate complex subtasks (e.g., vision processing, coding, math) to specialized models available locally.

### Implementation
- **Tool Injection**: The system injects a `chain_request` tool into the model's context (via `GetChainTools()` in `server/model_chain.go`).
- **Orchestration**: The `chain_request` tool acts as a bridge. If the primary model encounters a request requiring a capability it lacks (e.g., analyzing an image), it triggers the tool, specifying a `reason` and breaking the work into `sub_tasks`. 
- **Model Awareness**: When building the tool prompt, Ollama automatically fetches a list of locally available models, including their descriptions and capabilities. These are appended to the tool's description so the orchestrator model is fully aware of its environment and can supply a `preferred_model`.

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
