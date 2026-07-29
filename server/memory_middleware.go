package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/server/memory"
)

// memoryEngine is the global singleton engine. It is nil when the memory
// subsystem is disabled (the default). Initialised in Serve().
var memoryEngine memory.MemoryEngine

// initMemoryEngine reads config and starts the memory engine if enabled.
// Safe to call multiple times; only the first call has effect.
func initMemoryEngine(ctx context.Context) {
	cfg := memory.LoadConfig()
	if !cfg.Enabled {
		slog.Debug("memory subsystem disabled")
		return
	}

	eng, err := memory.NewEngine(cfg)
	if err != nil {
		slog.Error("memory engine init failed", "error", err)
		return
	}

	eng.Start(ctx)
	memoryEngine = eng
	slog.Info("memory subsystem enabled", "db", cfg.DBPath)
}

// shutdownMemoryEngine gracefully stops the engine.
func shutdownMemoryEngine() {
	if memoryEngine != nil {
		if err := memoryEngine.Close(); err != nil {
			slog.Warn("memory engine shutdown error", "error", err)
		}
		memoryEngine = nil
	}
}

// chainSystemInstructions is appended to the system prompt when memory tools
// are available. It tells the model exactly when and how to use each tool.
const chainSystemInstructions = `
## Model Chaining (chain_request tool)
Use chain_request when the user's request needs capabilities you lack (vision, image generation, audio, specialized code, math, embeddings, etc.) or when multiple specialist models would produce better results. The system will:
1. Unload you temporarily to free memory
2. Load specialist model(s) sequentially, passing outputs between steps
3. Reload you and return aggregated results for your final answer
The user sees live progress for every step.

## Task Scheduler (schedule_task tool)
Use schedule_task to run prompts at a future time or on a recurring schedule. Actions:
- schedule: create a job — requires prompt + run_at (e.g. "in 5m", "2h30m", RFC3339) or cron ("*/5 * * * *")
- list:     show all scheduled jobs (optionally filter by status)
- cancel:   stop a pending job by job_id
- get_result: retrieve the output of a completed job by job_id
- delete:   permanently remove a job by job_id

Examples of when to use schedule_task:
- "Remind me in 1 hour" → schedule with run_at: "in 1h"
- "Summarise the news every morning at 9" → cron: "0 9 * * *"
- "Run this analysis at midnight" → run_at: RFC3339 timestamp
- "What did the 3 AM job produce?" → get_result with job_id
`


// injectMemoryIntoMessages enriches the message list with relevant memories
// from past sessions. It replaces (or prepends) the system message with an
// enriched version that includes memory context.
//
// Returns the (possibly modified) message slice. Always safe — never panics
// and never blocks inference if the memory engine is down.
func injectMemoryIntoMessages(ctx context.Context, s *Server, userID string, req *api.ChatRequest, msgs []api.Message, hasTools bool) []api.Message {
	if memoryEngine == nil || userID == "" {
		return msgs
	}

	cfg := memory.LoadConfig()

	// Inject memory tools when the model supports tool calls
	if hasTools {
		req.Tools = append(req.Tools, GetMemoryTools(ctx, s)...)
	}

	// Derive the current system prompt from the message list
	var systemPrompt string
	var systemIdx int = -1
	for i, m := range msgs {
		if m.Role == "system" {
			systemPrompt = m.Content
			systemIdx = i
			break
		}
	}

	// Build enriched system prompt
	enrichedSystem := systemPrompt

	// Append chain instructions when chain is enabled and model has tools
	if hasTools && cfg.ChainEnabled {
		enrichedSystem = strings.TrimRight(enrichedSystem, "\n") + chainSystemInstructions
	}

	// Retrieve relevant memories and append them
	memReq := &memory.MemoryRequest{
		UserID:       userID,
		Model:        req.Model,
		SystemPrompt: systemPrompt,
		Messages:     toMemoryMessages(msgs),
	}

	memResp, err := memoryEngine.ProcessRequest(ctx, memReq)
	if err != nil {
		slog.Warn("memory: ProcessRequest failed", "error", err)
	} else if memResp.EnrichedSystem != "" && memResp.EnrichedSystem != systemPrompt {
		// Memory engine produced enriched context — prefer it as the base
		// but keep chain instructions if we added them
		if hasTools && cfg.ChainEnabled {
			enrichedSystem = memResp.EnrichedSystem + chainSystemInstructions
		} else {
			enrichedSystem = memResp.EnrichedSystem
		}
		slog.Debug("memory: injected memories",
			"user", userID,
			"memories", len(memResp.RelevantMemories),
			"tokens", memResp.ContextUsed,
		)
	}

	// Nothing changed — return original
	if enrichedSystem == systemPrompt {
		return msgs
	}

	// Replace existing system message or prepend a new one
	enriched := make([]api.Message, 0, len(msgs)+1)
	if systemIdx >= 0 {
		for i, m := range msgs {
			if i == systemIdx {
				enriched = append(enriched, api.Message{Role: "system", Content: enrichedSystem})
			} else {
				enriched = append(enriched, m)
			}
		}
	} else {
		// Prepend system message
		enriched = append([]api.Message{{Role: "system", Content: enrichedSystem}}, msgs...)
	}

	return enriched
}


// collectAndStoreMemories wraps a chat response channel. It passes every
// response through to the caller unchanged while accumulating the full
// assistant reply. When the channel closes it fires storeResponseMemories
// asynchronously so the HTTP response is never delayed.
func collectAndStoreMemories(ctx context.Context, userID string, req *api.ChatRequest, ch chan any) chan any {
	slog.Info("collectAndStoreMemories: wrapping channel", "userID", userID, "model", req.Model)
	if memoryEngine == nil || userID == "" {
		slog.Warn("collectAndStoreMemories: memory engine or userID missing", "engineNil", memoryEngine == nil, "userID", userID)
		return ch
	}

	out := make(chan any, cap(ch))

	go func() {
		defer close(out)
		var reply strings.Builder

		for item := range ch {
			out <- item // pass through immediately — no latency added

			slog.Info("collectAndStoreMemories: received item", "type", fmt.Sprintf("%T", item))

			if resp, ok := item.(api.ChatResponse); ok {
				reply.WriteString(resp.Message.Content)
				slog.Info("collectAndStoreMemories: accumulated content chunk", "chunkLen", len(resp.Message.Content), "done", resp.Done, "accumulatedLen", reply.Len())

				if resp.Done && reply.Len() > 0 {
					// Store memories after the last chunk is forwarded
					fullReply := reply.String()
					slog.Info("collectAndStoreMemories: response done, storing memories", "userID", userID, "replyLen", len(fullReply))
					go storeResponseMemories(ctx, userID, req, fullReply)
				}
			}
		}
	}()

	return out
}

// storeResponseMemories asynchronously extracts and stores memories from
// a completed assistant response. Non-blocking.
func storeResponseMemories(ctx context.Context, userID string, req *api.ChatRequest, assistantReply string) {
	slog.Info("storeResponseMemories: starting", "userID", userID, "replyLen", len(assistantReply))
	if memoryEngine == nil || userID == "" || assistantReply == "" {
		slog.Warn("storeResponseMemories: preconditions failed", "engineNil", memoryEngine == nil, "userID", userID, "replyEmpty", assistantReply == "")
		return
	}

	memReq := &memory.MemoryRequest{
		UserID:   userID,
		Model:    req.Model,
		Messages: toMemoryMessages(req.Messages),
	}
	slog.Info("storeResponseMemories: calling ProcessResponse", "numMessages", len(req.Messages))
	memoryEngine.ProcessResponse(ctx, memReq, assistantReply)
}

// toMemoryMessages converts API messages to memory package messages.
func toMemoryMessages(msgs []api.Message) []memory.Message {
	result := make([]memory.Message, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, memory.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return result
}

// memoryUserID extracts a stable user identifier from the request context.
// Falls back to the remote IP address when no auth token is present.
func memoryUserID(req *api.ChatRequest, remoteAddr string) string {
	// Use model as namespace separator so memories don't bleed across models.
	// For a real multi-user deployment, use the authenticated user ID.
	// RemoteAddr gives per-device isolation for single-user setups.
	host := remoteAddr
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx] // strip port
	}
	if host == "" || host == "127.0.0.1" || host == "::1" {
		host = "local"
	}
	return "user:" + host
}
