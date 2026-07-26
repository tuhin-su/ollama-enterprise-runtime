package server

import (
	"context"
	"log/slog"

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

// enrichChatRequestWithMemory injects relevant memories into the system
// message of a chat request. It returns the enriched system prompt string
// and the memory response (for use in ProcessResponse after generation).
//
// The function is a no-op when the memory engine is disabled or an error
// occurs — it never blocks inference.
func enrichChatRequestWithMemory(ctx context.Context, userID string, req *api.ChatRequest) (*memory.MemoryResponse, string) {
	if memoryEngine == nil {
		return nil, ""
	}

	// Build the memory request from the chat request
	memReq := &memory.MemoryRequest{
		UserID: userID,
		Model:  req.Model,
		Messages: toMemoryMessages(req.Messages),
	}

	// Extract existing system prompt
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			memReq.SystemPrompt = msg.Content
			break
		}
	}

	memResp, err := memoryEngine.ProcessRequest(ctx, memReq)
	if err != nil {
		slog.Warn("memory: ProcessRequest failed", "error", err)
		return nil, ""
	}

	return memResp, memReq.SystemPrompt
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
