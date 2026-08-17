package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/loom/loom/api"
)

// ConversationalRewriter rewrites follow-up queries using conversation context or HyDE.
type ConversationalRewriter struct{}

func NewConversationalRewriter() *ConversationalRewriter {
	return &ConversationalRewriter{}
}

// RewriteQuery transforms ambiguous follow-up questions into standalone search queries.
func (r *ConversationalRewriter) RewriteQuery(query string, history []Message) string {
	if len(history) == 0 {
		return query
	}

	// Extract the last 2 turns of conversation context
	var contextParts []string
	start := len(history) - 4
	if start < 0 {
		start = 0
	}
	for i := start; i < len(history); i++ {
		contextParts = append(contextParts, fmt.Sprintf("%s: %s", history[i].Role, history[i].Content))
	}

	contextStr := strings.Join(contextParts, " | ")
	
	// If query has ambiguous pronouns or short length, append conversational context
	queryLower := strings.ToLower(query)
	ambiguousTokens := []string{"that", "it", "this", "they", "them", "the experiment", "the result", "the model", "he", "she"}
	
	isAmbiguous := false
	for _, token := range ambiguousTokens {
		if strings.Contains(queryLower, token) {
			isAmbiguous = true
			break
		}
	}

	if isAmbiguous {
		return fmt.Sprintf("%s (Context: %s)", query, contextStr)
	}

	return query
}

// GenerateHyDEDocument generates a hypothetical response document to query against vector storage.
func (r *ConversationalRewriter) GenerateHyDEDocument(query string) string {
	return fmt.Sprintf("Hypothetical document answering: %s\nSummary of facts, findings, and details related to %s.", query, query)
}

// SelfModifyingMemory allows the model to modify, pin, update, or self-organize its own memory records.
type SelfModifyingMemoryStore struct {
	store MemoryStore
}

func NewSelfModifyingMemoryStore(store MemoryStore) *SelfModifyingMemoryStore {
	return &SelfModifyingMemoryStore{store: store}
}

// UpdateMemory performs an in-place update of an existing memory content, importance, or tags.
func (s *SelfModifyingMemoryStore) UpdateMemory(ctx context.Context, id string, newContent string, newImportance float64, newTags []string) error {
	mems, err := s.store.GetByIDs(ctx, []string{id})
	if err != nil || len(mems) == 0 {
		return fmt.Errorf("memory %s not found: %v", id, err)
	}

	mem := mems[0]
	if newContent != "" {
		mem.Content = newContent
	}
	if newImportance > 0 {
		mem.Importance = newImportance
	}
	if len(newTags) > 0 {
		mem.Tags = newTags
	}
	mem.UpdatedAt = time.Now()

	return s.store.Save(ctx, mem)
}

// PinMemory marks a memory as pinned so it is always prioritized during search.
func (s *SelfModifyingMemoryStore) PinMemory(ctx context.Context, id string, pinned bool) error {
	mems, err := s.store.GetByIDs(ctx, []string{id})
	if err != nil || len(mems) == 0 {
		return fmt.Errorf("memory %s not found: %v", id, err)
	}

	mem := mems[0]
	mem.Pinned = pinned
	mem.UpdatedAt = time.Now()

	return s.store.Save(ctx, mem)
}

// GetSelfModifyingTools returns tool definitions enabling models to self-manage their memory.
func GetSelfModifyingTools() api.Tools {
	updateProps := api.NewToolPropertiesMap()
	updateProps.Set("id", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The unique ID of the memory to update.",
	})
	updateProps.Set("content", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Updated memory content string.",
	})
	updateProps.Set("importance", api.ToolProperty{
		Type:        api.PropertyType{"number"},
		Description: "Updated importance score (0.0 - 1.0).",
	})

	pinProps := api.NewToolPropertiesMap()
	pinProps.Set("id", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The memory ID to pin or unpin.",
	})
	pinProps.Set("pinned", api.ToolProperty{
		Type:        api.PropertyType{"boolean"},
		Description: "True to pin memory, false to unpin.",
	})

	return api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "update_memory",
				Description: "Self-modify or update an existing memory content or importance score.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: updateProps,
					Required:   []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "pin_memory",
				Description: "Pin an important memory so it is always prioritized in prompt context.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: pinProps,
					Required:   []string{"id", "pinned"},
				},
			},
		},
	}
}
