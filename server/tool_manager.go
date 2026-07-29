package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/server/memory"
)

// ToolManager handles the volatile RAG database for dynamic tool retrieval.
type ToolManager struct {
	store *memory.LanceDBStore
	index *memory.BruteForceIndex
	dbDir string
}

var globalToolManager *ToolManager

// InitToolManager initializes the volatile LanceDB vector database for tools.
// It explicitly wipes the directory to ensure no stale disconnected tools remain.
func InitToolManager(ctx context.Context, s *Server) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dbDir := filepath.Join(home, ".ollama", "toolsmanager_db")

	// Volatile: Wipe it clean on startup
	_ = os.RemoveAll(dbDir)

	store, err := memory.NewLanceDBStore(dbDir)
	if err != nil {
		return fmt.Errorf("failed to initialize toolsmanager_db: %w", err)
	}

	globalToolManager = &ToolManager{
		store: store,
		index: memory.NewBruteForceIndex(),
		dbDir: dbDir,
	}

	slog.Info("ToolManager initialized with volatile vector DB", "path", dbDir)

	// Register built-in tools (Memory & Chain) dynamically instead of bloating context
	globalToolManager.RegisterBuiltinTools(ctx, s)

	return nil
}

func (tm *ToolManager) RegisterBuiltinTools(ctx context.Context, s *Server) {
	if memoryEngine == nil {
		return // Cannot generate embeddings without memory engine
	}

	var builtinTools []api.Tool
	builtinTools = append(builtinTools, GetMemoryTools(ctx, s)...)
	builtinTools = append(builtinTools, GetChainTools(ctx, s)...)

	for _, tool := range builtinTools {
		// Suppress logs for builtins so we don't spam startup
		_ = tm.AddTool(ctx, tool.Function.Name, tool)
	}
	
	slog.Info("ToolManager registered built-in tools natively", "count", len(builtinTools))
}

// AddTool embeds a tool's description and adds it to the vector index and store.
func (tm *ToolManager) AddTool(ctx context.Context, toolName string, schema api.Tool) error {
	if memoryEngine == nil {
		return fmt.Errorf("memory engine is required to generate embeddings for tools")
	}

	// 1. Generate embedding for the tool description
	desc := schema.Function.Description
	if desc == "" {
		desc = schema.Function.Name
	}
	emb, err := memoryEngine.Embed(ctx, desc)
	if err != nil {
		return fmt.Errorf("failed to embed tool %s: %w", toolName, err)
	}

	// 2. Serialize schema to JSON
	schemaJSON, _ := json.Marshal(schema)

	// 3. Save to LanceDB (using SpecialMemory as a convenient key-value store)
	specMem := &memory.SpecialMemory{
		ID:        toolName,
		UserID:    "system_tools",
		Key:       "tool_schema",
		Value:     string(schemaJSON),
		Embedding: emb,
	}

	if err := tm.store.SaveSpecialMemory(ctx, specMem); err != nil {
		return fmt.Errorf("failed to save tool to LanceDB: %w", err)
	}

	// 4. Add to in-memory vector index for fast kNN search
	if err := tm.index.Add(toolName, emb); err != nil {
		return fmt.Errorf("failed to add tool to vector index: %w", err)
	}

	slog.Info("Tool registered in vector DB", "tool", toolName)
	return nil
}

// RemoveTool deletes a tool from the vector index and store.
func (tm *ToolManager) RemoveTool(ctx context.Context, toolName string) {
	_ = tm.index.Remove(toolName)
	_ = tm.store.DeleteSpecialMemory(ctx, toolName)
	slog.Info("Tool removed from vector DB", "tool", toolName)
}

// SearchTools performs a kNN search against connected tools based on a query.
func (tm *ToolManager) SearchTools(ctx context.Context, query string, topK int) ([]api.Tool, error) {
	if memoryEngine == nil {
		return nil, fmt.Errorf("memory engine disabled")
	}

	emb, err := memoryEngine.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	results, err := tm.index.Search(emb, topK)
	if err != nil || len(results) == 0 {
		return nil, err
	}

	// Fetch JSON schemas for the matching tools from the DB
	var tools []api.Tool
	for _, res := range results {
		// Only consider reasonable matches
		if res.Similarity < 0.3 {
			continue
		}
		
		memories, err := tm.store.ListSpecialMemories(ctx, "system_tools")
		if err == nil {
			for _, m := range memories {
				if m.ID == res.ID {
					var t api.Tool
					if err := json.Unmarshal([]byte(m.Value), &t); err == nil {
						tools = append(tools, t)
					}
					break
				}
			}
		}
	}

	return tools, nil
}
