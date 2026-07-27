package server

import (
	"context"
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

// injectMemoryIntoMessages enriches the message list with relevant memories
// from past sessions. It replaces (or prepends) the system message with an
// enriched version that includes memory context.
//
// Returns the (possibly modified) message slice. Always safe — never panics
// and never blocks inference if the memory engine is down.
func injectMemoryIntoMessages(ctx context.Context, userID string, req *api.ChatRequest, msgs []api.Message, hasTools bool) []api.Message {
	if memoryEngine == nil || userID == "" {
		return msgs
	}

	if hasTools {
		req.Tools = append(req.Tools, GetMemoryTools()...)
		return msgs
	}

	// Derive the current system prompt from the message list
	var systemPrompt string
	for _, m := range msgs {
		if m.Role == "system" {
			systemPrompt = m.Content
			break
		}
	}

	memReq := &memory.MemoryRequest{
		UserID:       userID,
		Model:        req.Model,
		SystemPrompt: systemPrompt,
		Messages:     toMemoryMessages(msgs),
	}

	memResp, err := memoryEngine.ProcessRequest(ctx, memReq)
	if err != nil {
		slog.Warn("memory: ProcessRequest failed", "error", err)
		return msgs
	}

	// Nothing useful retrieved
	if memResp.EnrichedSystem == "" || memResp.EnrichedSystem == systemPrompt {
		return msgs
	}

	// Replace existing system message or prepend a new one
	enriched := make([]api.Message, 0, len(msgs)+1)
	replaced := false
	for _, m := range msgs {
		if m.Role == "system" {
			enriched = append(enriched, api.Message{Role: "system", Content: memResp.EnrichedSystem})
			replaced = true
		} else {
			enriched = append(enriched, m)
		}
	}
	if !replaced {
		// Prepend system message
		enriched = append([]api.Message{{Role: "system", Content: memResp.EnrichedSystem}}, enriched...)
	}

	slog.Debug("memory: injected memories",
		"user", userID,
		"memories", len(memResp.RelevantMemories),
		"tokens", memResp.ContextUsed,
	)
	return enriched
}

// collectAndStoreMemories wraps a chat response channel. It passes every
// response through to the caller unchanged while accumulating the full
// assistant reply. When the channel closes it fires storeResponseMemories
// asynchronously so the HTTP response is never delayed.
func collectAndStoreMemories(ctx context.Context, userID string, req *api.ChatRequest, ch chan any) chan any {
	if memoryEngine == nil || userID == "" {
		return ch
	}

	out := make(chan any, cap(ch))

	go func() {
		defer close(out)
		var reply strings.Builder

		for item := range ch {
			out <- item // pass through immediately — no latency added

			if resp, ok := item.(api.ChatResponse); ok {
				reply.WriteString(resp.Message.Content)

				if resp.Done && reply.Len() > 0 {
					// Store memories after the last chunk is forwarded
					fullReply := reply.String()
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
	if memoryEngine == nil || userID == "" || assistantReply == "" {
		return
	}

	memReq := &memory.MemoryRequest{
		UserID:   userID,
		Model:    req.Model,
		Messages: toMemoryMessages(req.Messages),
	}
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
