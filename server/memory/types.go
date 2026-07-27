// Package memory provides a native high-performance long-term memory system
// for Ollama. It persists user facts, project context, conversation summaries,
// episodic events, and semantic embeddings across sessions.
package memory

import (
	"context"
	"time"
)

// MemoryType categorises a memory entry.
type MemoryType string

const (
	MemoryTypeUser         MemoryType = "user"
	MemoryTypeProject      MemoryType = "project"
	MemoryTypeConversation MemoryType = "conversation"
	MemoryTypeEpisodic     MemoryType = "episodic"
	MemoryTypeSemantic     MemoryType = "semantic"
)

// Memory is the fundamental unit of persistent knowledge.
type Memory struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Type         MemoryType `json:"type"`
	Content      string     `json:"content"`
	Summary      string     `json:"summary,omitempty"`
	Importance   float64    `json:"importance"`
	Embedding    []float32  `json:"embedding,omitempty"`
	Tags         []string   `json:"tags,omitempty"`
	AccessCount  int64      `json:"access_count"`
	Pinned       bool       `json:"pinned"`
	Archived     bool       `json:"archived"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastAccessed time.Time  `json:"last_accessed"`
}

// Conversation represents a chat history turn.
type Conversation struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	Model            string    `json:"model"`
	UserMessage      string    `json:"user_message"`
	AssistantMessage string    `json:"assistant_message"`
	Thinking         string    `json:"thinking,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
}

// SpecialMemory represents a memory block managed directly by the AI.
type SpecialMemory struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Embedding []float32 `json:"embedding,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SearchResult pairs a memory with its computed relevance score.
type SearchResult struct {
	Memory     *Memory `json:"memory"`
	Score      float64 `json:"score"`
	Similarity float64 `json:"similarity"`
}

// VectorResult is returned by the raw vector index.
type VectorResult struct {
	ID         string  `json:"id"`
	Similarity float64 `json:"similarity"`
}

// ExtractedMemory is a candidate memory detected in a model response.
type ExtractedMemory struct {
	Content    string     `json:"content"`
	Type       MemoryType `json:"type"`
	Importance float64    `json:"importance"`
	Tags       []string   `json:"tags,omitempty"`
}

// ListOptions controls filtering for store listing.
type ListOptions struct {
	Type     MemoryType
	Pinned   *bool
	Archived *bool
	Limit    int
	Offset   int
}

// SearchOptions controls semantic search parameters.
type SearchOptions struct {
	Query               string
	TopK                int
	SimilarityThreshold float64
	Type                MemoryType
	IncludeArchived     bool
}

// MemoryRequest carries all context needed to process a single turn.
type MemoryRequest struct {
	UserID         string
	Model          string
	ConversationID string
	Messages       []Message
	SystemPrompt   string
}

// MemoryResponse contains the enriched prompt data returned to the caller.
type MemoryResponse struct {
	RelevantMemories []*SearchResult
	EnrichedSystem   string
	ContextUsed      int // estimated tokens consumed by memory
}

// Message is a simplified chat message used by the memory subsystem.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ---------------------------------------------------------------------------
// Interfaces — every component is replaceable.
// ---------------------------------------------------------------------------

// MemoryStore is the persistent storage backend.
type MemoryStore interface {
	Save(ctx context.Context, mem *Memory) error
	Get(ctx context.Context, id string) (*Memory, error)
	List(ctx context.Context, userID string, opts ListOptions) ([]*Memory, error)
	Update(ctx context.Context, mem *Memory) error
	Delete(ctx context.Context, id string) error
	IncrementAccess(ctx context.Context, id string) error
	GetByIDs(ctx context.Context, ids []string) ([]*Memory, error)
	CountByUser(ctx context.Context, userID string) (int64, error)
	ArchiveOlderThan(ctx context.Context, before time.Time, minImportance float64) (int64, error)
	Close() error
	// Conversation methods
	SaveConversation(ctx context.Context, conv *Conversation) error
	// SpecialMemory methods
	SaveSpecialMemory(ctx context.Context, mem *SpecialMemory) error
	ListSpecialMemories(ctx context.Context, userID string) ([]*SpecialMemory, error)
	DeleteSpecialMemory(ctx context.Context, id string) error
	// Export/Import helper
	Export(ctx context.Context) ([]*Memory, []*Conversation, []*SpecialMemory, error)
	Wipe(ctx context.Context) error
}

// VectorIndex performs approximate nearest-neighbour search on embeddings.
type VectorIndex interface {
	Add(id string, embedding []float32) error
	Remove(id string) error
	Search(query []float32, topK int) ([]VectorResult, error)
	Size() int
	Close() error
}

// EmbeddingProvider turns text into dense vectors.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}

// MemoryExtractor analyses model output and returns candidate memories.
type MemoryExtractor interface {
	Extract(ctx context.Context, userMsg, assistantMsg string) ([]ExtractedMemory, error)
}

// PromptBuilder constructs the final system prompt with injected memories.
type PromptBuilder interface {
	Build(memories []*SearchResult, systemPrompt string, maxTokens int) string
}

// CacheProvider is the hot-path cache sitting in front of the store.
type CacheProvider interface {
	Get(key string) (any, bool)
	Set(key string, value any, cost int64) bool
	SetWithTTL(key string, value any, cost int64, ttl time.Duration) bool
	Del(key string)
	Clear()
	Close()
}

// MemoryEngine is the top-level orchestrator exposed to the server.
type MemoryEngine interface {
	// ProcessRequest enriches an incoming request with relevant memories.
	ProcessRequest(ctx context.Context, req *MemoryRequest) (*MemoryResponse, error)
	// ProcessResponse extracts and stores new memories from a model response.
	ProcessResponse(ctx context.Context, req *MemoryRequest, response string)
	// Healthy returns nil when the engine is operational.
	Healthy(ctx context.Context) error
	// Close shuts down background workers and releases resources.
	Close() error
	// Store returns the underlying persistent memory store.
	Store() MemoryStore
	// Embed generates an embedding for a piece of text using the engine's embedder.
	Embed(ctx context.Context, text string) ([]float32, error)
}
