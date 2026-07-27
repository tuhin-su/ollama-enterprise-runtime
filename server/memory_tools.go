package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/server/memory"
)

// GetMemoryTools returns the list of memory-related tools.
func GetMemoryTools() api.Tools {
	// save_memory properties
	saveProps := api.NewToolPropertiesMap()
	saveProps.Set("content", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The clear, concise fact or information to remember (e.g., 'User prefers Python over Go').",
	})
	saveProps.Set("type", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The category of memory: 'user', 'project', 'conversation', 'episodic', or 'semantic'.",
		Enum:        []any{"user", "project", "conversation", "episodic", "semantic"},
	})
	saveProps.Set("tags", api.ToolProperty{
		Type:        api.PropertyType{"array"},
		Description: "Optional tags to group this memory.",
		Items: api.ToolProperty{
			Type: api.PropertyType{"string"},
		},
	})

	// list_memories properties
	listProps := api.NewToolPropertiesMap()
	listProps.Set("type", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Optional filter by category.",
		Enum:        []any{"user", "project", "conversation", "episodic", "semantic"},
	})
	listProps.Set("pinned", api.ToolProperty{
		Type:        api.PropertyType{"boolean"},
		Description: "Optional filter by pinned status.",
	})
	listProps.Set("archived", api.ToolProperty{
		Type:        api.PropertyType{"boolean"},
		Description: "Optional filter by archived status.",
	})

	// delete_memory properties
	deleteProps := api.NewToolPropertiesMap()
	deleteProps.Set("id", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The unique ID of the memory to delete.",
	})

	// save_special_memory properties (AI managed special table)
	saveSpecialProps := api.NewToolPropertiesMap()
	saveSpecialProps.Set("key", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The key or topic for this special memory.",
	})
	saveSpecialProps.Set("value", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The data or value content for this special memory.",
	})

	// delete_special_memory properties
	deleteSpecialProps := api.NewToolPropertiesMap()
	deleteSpecialProps.Set("id", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The unique ID of the special memory to delete.",
	})

	return api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "save_memory",
				Description: "Save a new fact, context, or piece of information to long-term memory.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: saveProps,
					Required:   []string{"content"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "list_memories",
				Description: "Search, filter, or retrieve saved memories.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: listProps,
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "delete_memory",
				Description: "Delete a memory by its unique ID.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: deleteProps,
					Required:   []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "save_special_memory",
				Description: "Save a piece of key-value data to the AI's special memory table managed entirely by the AI.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: saveSpecialProps,
					Required:   []string{"key", "value"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "list_special_memories",
				Description: "List all items in the AI's special memory table.",
				Parameters: api.ToolFunctionParameters{
					Type: "object",
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "delete_special_memory",
				Description: "Delete an item from the AI's special memory table by ID.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: deleteSpecialProps,
					Required:   []string{"id"},
				},
			},
		},
	}
}

// ExecuteMemoryTool runs a memory tool against the LanceDB store and returns the result as string/JSON.
func ExecuteMemoryTool(ctx context.Context, userID string, toolCall api.ToolCall) (string, error) {
	if memoryEngine == nil {
		return "", fmt.Errorf("memory engine not initialized")
	}

	args := toolCall.Function.Arguments.ToMap()
	store := memoryEngine.Store()

	switch toolCall.Function.Name {
	case "save_memory":
		content, ok := args["content"].(string)
		if !ok || content == "" {
			return "", fmt.Errorf("content is required")
		}
		memType := memory.MemoryTypeSemantic
		if t, ok := args["type"].(string); ok && t != "" {
			memType = memory.MemoryType(t)
		}
		var tags []string
		if tSlice, ok := args["tags"].([]any); ok {
			for _, tVal := range tSlice {
				if ts, ok := tVal.(string); ok {
					tags = append(tags, ts)
				}
			}
		} else if tSliceStr, ok := args["tags"].([]string); ok {
			tags = tSliceStr
		}

		emb, err := memoryEngine.Embed(ctx, content)
		if err != nil {
			return "", fmt.Errorf("failed to generate embedding: %w", err)
		}

		mem := &memory.Memory{
			UserID:     userID,
			Type:       memType,
			Content:    content,
			Embedding:  emb,
			Tags:       tags,
			Importance: 0.8,
		}

		if err := store.Save(ctx, mem); err != nil {
			return "", fmt.Errorf("failed to save memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     mem.ID,
			"msg":    "Memory successfully saved.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "list_memories":
		opts := memory.ListOptions{}
		if t, ok := args["type"].(string); ok && t != "" {
			opts.Type = memory.MemoryType(t)
		}
		if p, ok := args["pinned"].(bool); ok {
			opts.Pinned = &p
		}
		if a, ok := args["archived"].(bool); ok {
			opts.Archived = &a
		}

		memories, err := store.List(ctx, userID, opts)
		if err != nil {
			return "", fmt.Errorf("failed to list memories: %w", err)
		}

		type SimpleMem struct {
			ID      string            `json:"id"`
			Type    memory.MemoryType `json:"type"`
			Content string            `json:"content"`
			Tags    []string          `json:"tags,omitempty"`
		}
		simpleMems := make([]SimpleMem, len(memories))
		for i, m := range memories {
			simpleMems[i] = SimpleMem{
				ID:      m.ID,
				Type:    m.Type,
				Content: m.Content,
				Tags:    m.Tags,
			}
		}

		resBytes, _ := json.Marshal(simpleMems)
		return string(resBytes), nil

	case "delete_memory":
		id, ok := args["id"].(string)
		if !ok || id == "" {
			return "", fmt.Errorf("id is required")
		}

		if err := store.Delete(ctx, id); err != nil {
			return "", fmt.Errorf("failed to delete memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     id,
			"msg":    "Memory successfully deleted.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "save_special_memory":
		key, ok := args["key"].(string)
		if !ok || key == "" {
			return "", fmt.Errorf("key is required")
		}
		value, ok := args["value"].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("value is required")
		}

		// Generate embedding for the special memory value
		emb, err := memoryEngine.Embed(ctx, fmt.Sprintf("%s: %s", key, value))
		if err != nil {
			return "", fmt.Errorf("failed to generate embedding: %w", err)
		}

		mem := &memory.SpecialMemory{
			UserID:    userID,
			Key:       key,
			Value:     value,
			Embedding: emb,
		}

		if err := store.SaveSpecialMemory(ctx, mem); err != nil {
			return "", fmt.Errorf("failed to save special memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     mem.ID,
			"msg":    "Special memory successfully saved.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "list_special_memories":
		memories, err := store.ListSpecialMemories(ctx, userID)
		if err != nil {
			return "", fmt.Errorf("failed to list special memories: %w", err)
		}

		resBytes, _ := json.Marshal(memories)
		return string(resBytes), nil

	case "delete_special_memory":
		id, ok := args["id"].(string)
		if !ok || id == "" {
			return "", fmt.Errorf("id is required")
		}

		if err := store.DeleteSpecialMemory(ctx, id); err != nil {
			return "", fmt.Errorf("failed to delete special memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     id,
			"msg":    "Special memory successfully deleted.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	default:
		return "", fmt.Errorf("unknown memory tool: %s", toolCall.Function.Name)
	}
}

// IsMemoryTool returns true if the tool name belongs to one of the memory tools.
func IsMemoryTool(name string) bool {
	return name == "save_memory" || name == "list_memories" || name == "delete_memory" ||
		name == "save_special_memory" || name == "list_special_memories" || name == "delete_special_memory"
}
