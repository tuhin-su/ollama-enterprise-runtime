# Session Engineering Log & Conversation History

**Date:** July 29, 2026  
**Project:** Loom Dynamic Agent & RAG Runtime  
**Workspace:** `/home/master/Desktop/loom-master`

---

## 1. Executive Summary & Context
During this engineering session, we addressed key logic holes, bootstrap deadlocks, and critical security vulnerabilities in the Loom codebase. We transitioned the codebase from a capability-first prototype to a secure, concurrent, and highly robust local AI runtime capable of orchestrating server-side memory and external WebSocket tools dynamically without hanging or deadlocking.

---

## 2. Issues Identified & Resolved

### Bug 1: Dynamic WebSocket Tools Ignored by Inference Loop
* **Symptom:** WebSocket-connected custom tools (like `python_sandbox` or `data_analyzer`) registered successfully in the vector database but were never executed when invoked by the LLM.
* **Root Cause:** In `routes.go`, the handler checked a strict whitelist: `IsMemoryTool(tc.Function.Name)`. Because WebSocket-connected custom tools were not in this hardcoded whitelist, they were filtered out and ignored.
* **Remediation:** Wired synchronous Go-to-WebSocket bridging in `server/tool_interface.go` using a map of channels (`PendingHTTPRequests`). Modified `routes.go` to intercept both memory tools and connected WebSocket tools, routing them cleanly via `executeToolOrMemory`.

### Bug 2: Server Startup Bootstrap Deadlock
* **Symptom:** Starting `./loom serve` hung indefinitely, causing external tool connections to fail with handshake timeouts.
* **Root Cause:** `InitToolManager` registered built-in memory/chaining tools on startup, which required calculating embeddings. The embedding client made HTTP requests to `/api/embed` on the local port (`11434`). However, the HTTP server had not started listening yet because it was waiting for `InitToolManager` to finish.
* **Remediation:** Refactored `server/tool_manager.go` to run the `RegisterBuiltinTools` routine in a background goroutine with a 1-second delay, allowing the server to bind port `11434` immediately and break the deadlock.

### Bug 3: Jinja Chat Template Prompts Exception
* **Symptom:** Chat request failed with: `Jinja Exception: System message must be at the beginning.`
* **Root Cause:** Adding memory contexts and dynamic system notes created multiple `Role: "system"` messages located in different parts of the message list (some in the middle), violating Jinja's strict template rules.
* **Remediation:** Wrote `MergeSystemMessages(msgs)` in `routes.go` to scan the list, consolidate all system prompts into a single clean system instruction, and place it at index `0`.

### Bug 4: Server-Side Tool JSON Leaked to Client CLI
* **Symptom:** Chatting with tools returned empty outputs in the CLI.
* **Root Cause:** When the LLM called a tool, the streaming loop did not suppress the tool call JSON if the tool was not a memory tool. The raw JSON tool call was streamed, which the `loom` CLI discarded.
* **Remediation:** Updated the streaming suppression filter to suppress all server-executed tools (memory, WebSocket, and `toolmanager.search`), ensuring the client only receives the final generated text answer.

### Bug 5: Recursive Read-Lock Deadlocks
* **Symptom:** Chat queries involving tools hung and returned empty responses.
* **Root Cause:** The `VRAMArbiter.RLock()` read locks were acquired recursively in the same goroutine for each turn of the tool loop. Because Go's `sync.RWMutex` does not permit recursive read locking when a writer is waiting, this deadlocked the thread.
* **Remediation:** Moved the `VRAMArbiter.RLock()` to the outer goroutine of `handleNativeChat` so it is acquired once at the beginning of the API request and released once at the end.

---

## 3. Structural Security Hardening (Tier 3 Gate)
To remediate prompt injection vulnerabilities (untrusted pages instructing the model to execute destructive code), we implemented a **Privilege Ring Security Gate**:
- High-risk tools (`python_sandbox`, `restart_server`, `read_system_logs`, `system_tool`) are classified as **Tier 3**.
- The server rejects Tier 3 calls with a `403 Forbidden` security violation unless an explicit `"user_confirmed": true` parameter is passed in the arguments.
- Tool schemas (in `python_sandbox.py` and `memory_tools.go`) were updated to require this parameter so the model knows how to request explicit confirmation.

---

## 4. Shell Interface Upgrade (`chat.py`)
Upgraded the CLI chat interface with:
- Dynamic model selection menu.
- Real-time Markdown token streaming.
- Host details and dynamic tools table.
- Extended slash commands (`/help`, `/clear`, `/history`, `/info`, `/system`, `/image`).
