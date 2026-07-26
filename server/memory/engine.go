package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Engine is the production implementation of MemoryEngine.
// It wires together the store, cache, vector index, ranker, embedder,
// extractor, prompt builder, and background worker pool.
type Engine struct {
	cfg     Config
	store   MemoryStore
	cache   CacheProvider
	index   VectorIndex
	ranker  *Ranker
	embedder EmbeddingProvider
	extractor MemoryExtractor
	builder  PromptBuilder
	decay    *DecayProcessor
	pool     *WorkerPool

	mu      sync.RWMutex
	started bool
}

// NewEngine constructs an Engine from the given Config, using production
// implementations for every component.
func NewEngine(cfg Config) (*Engine, error) {
	// SQLite store
	store, err := NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("memory engine: store: %w", err)
	}

	// Ristretto cache
	cache, err := NewRistrettoCache(cfg.CacheSize*10, cfg.CacheMaxCost, cfg.CacheTTL.Duration)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("memory engine: cache: %w", err)
	}

	index := NewBruteForceIndex()
	ranker := NewRanker(cfg.Ranking)
	embedder := NewOllamaEmbedder(cfg.EmbeddingModel, "")
	extractor := NewPatternExtractor()
	builder := NewPromptBuilder()
	decay := NewDecayProcessor(store, index, cache, cfg)
	pool := NewWorkerPool(cfg.WorkerCount)

	e := &Engine{
		cfg:       cfg,
		store:     store,
		cache:     cache,
		index:     index,
		ranker:    ranker,
		embedder:  embedder,
		extractor: extractor,
		builder:   builder,
		decay:     decay,
		pool:      pool,
	}
	return e, nil
}

// Start launches the background worker pool and the periodic decay ticker.
// Must be called once after NewEngine.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return
	}
	e.started = true

	e.pool.Start()

	// Load existing embeddings into the vector index
	e.pool.Submit(func() {
		if err := e.warmIndex(ctx); err != nil {
			slog.Warn("memory engine: index warm failed", "error", err)
		}
	})

	// Periodic decay / archival
	if e.cfg.DecayIntervalHours > 0 {
		go e.decayLoop(ctx)
	}

	slog.Info("memory engine started",
		"db", e.cfg.DBPath,
		"embedding_model", e.cfg.EmbeddingModel,
		"workers", e.cfg.WorkerCount,
	)
}

// ProcessRequest enriches an incoming inference request with relevant memories.
// It is called on the hot path — all heavy work is cached.
func (e *Engine) ProcessRequest(ctx context.Context, req *MemoryRequest) (*MemoryResponse, error) {
	if req.UserID == "" {
		return &MemoryResponse{EnrichedSystem: req.SystemPrompt}, nil
	}

	// Build query text from the most recent user message
	queryText := lastUserMessage(req.Messages)
	if queryText == "" {
		return &MemoryResponse{EnrichedSystem: req.SystemPrompt}, nil
	}

	// Check search cache
	cacheKey := SearchCacheKey(req.UserID, queryText)
	if cached, ok := e.cache.Get(cacheKey); ok {
		if results, ok := cached.([]*SearchResult); ok {
			enriched := e.builder.Build(results[:minInt(len(results), e.cfg.MaxPromptMemories)],
				req.SystemPrompt, e.cfg.MaxPromptTokens)
			return &MemoryResponse{
				RelevantMemories: results,
				EnrichedSystem:   enriched,
				ContextUsed:      estimateTokens(enriched) - estimateTokens(req.SystemPrompt),
			}, nil
		}
	}

	// Embed the query
	queryEmb, err := e.embedder.Embed(ctx, queryText)
	if err != nil {
		// Non-fatal: return unenriched prompt
		slog.Warn("memory engine: embed query failed", "error", err)
		return &MemoryResponse{EnrichedSystem: req.SystemPrompt}, nil
	}

	// Vector search
	vecResults, err := e.index.Search(queryEmb, e.cfg.TopK)
	if err != nil || len(vecResults) == 0 {
		return &MemoryResponse{EnrichedSystem: req.SystemPrompt}, nil
	}

	// Filter by similarity threshold
	var ids []string
	sims := make(map[string]float64, len(vecResults))
	for _, vr := range vecResults {
		if vr.Similarity >= e.cfg.SimilarityThreshold {
			ids = append(ids, vr.ID)
			sims[vr.ID] = vr.Similarity
		}
	}
	if len(ids) == 0 {
		return &MemoryResponse{EnrichedSystem: req.SystemPrompt}, nil
	}

	// Fetch memories from store
	memories, err := e.store.GetByIDs(ctx, ids)
	if err != nil {
		slog.Warn("memory engine: get_by_ids failed", "error", err)
		return &MemoryResponse{EnrichedSystem: req.SystemPrompt}, nil
	}

	// Rank
	results := e.ranker.Rank(memories, sims)
	if len(results) > e.cfg.MaxPromptMemories {
		results = results[:e.cfg.MaxPromptMemories]
	}

	// Cache the results
	e.cache.SetWithTTL(cacheKey, results, int64(len(results)), 2*time.Minute)

	// Async: increment access counts
	memIDs := ids
	e.pool.Submit(func() {
		for _, id := range memIDs {
			_ = e.store.IncrementAccess(context.Background(), id)
		}
	})

	enriched := e.builder.Build(results, req.SystemPrompt, e.cfg.MaxPromptTokens)
	return &MemoryResponse{
		RelevantMemories: results,
		EnrichedSystem:   enriched,
		ContextUsed:      estimateTokens(enriched) - estimateTokens(req.SystemPrompt),
	}, nil
}

// ProcessResponse extracts new memories from the assistant's reply and stores
// them asynchronously. Never blocks the request path.
func (e *Engine) ProcessResponse(ctx context.Context, req *MemoryRequest, response string) {
	if req.UserID == "" || response == "" {
		return
	}

	userMsg := lastUserMessage(req.Messages)

	e.pool.Submit(func() {
		extracted, err := e.extractor.Extract(context.Background(), userMsg, response)
		if err != nil || len(extracted) == 0 {
			return
		}

		for _, em := range extracted {
			if em.Importance < e.cfg.ImportanceThreshold {
				continue
			}

			mem := &Memory{
				UserID:     req.UserID,
				Type:       em.Type,
				Content:    em.Content,
				Importance: em.Importance,
				Tags:       em.Tags,
			}

			// Generate embedding
			emb, err := e.embedder.Embed(context.Background(), em.Content)
			if err == nil {
				mem.Embedding = emb
			}

			if err := e.store.Save(context.Background(), mem); err != nil {
				slog.Warn("memory engine: save failed", "error", err)
				continue
			}

			// Index the vector
			if len(mem.Embedding) > 0 {
				if err := e.index.Add(mem.ID, mem.Embedding); err != nil {
					slog.Warn("memory engine: index add failed", "id", mem.ID, "error", err)
				}
			}

			// Invalidate user search cache
			e.cache.Del(UserCacheKey(req.UserID))
		}
	})
}

// Healthy returns nil if the engine is operational.
func (e *Engine) Healthy(_ context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.started {
		return fmt.Errorf("memory engine not started")
	}
	return nil
}

// Close shuts down the engine gracefully.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.pool.Stop()
	e.cache.Close()

	if err := e.index.Close(); err != nil {
		slog.Warn("memory engine: index close", "error", err)
	}
	return e.store.Close()
}

// -- internal helpers --

// warmIndex loads all non-archived embeddings into the in-process vector index.
func (e *Engine) warmIndex(ctx context.Context) error {
	archived := false
	memories, err := e.store.List(ctx, "", ListOptions{
		Archived: &archived,
		Limit:    100_000,
	})
	if err != nil {
		return err
	}

	count := 0
	for _, mem := range memories {
		if len(mem.Embedding) > 0 {
			if err := e.index.Add(mem.ID, mem.Embedding); err == nil {
				count++
			}
		}
	}
	slog.Info("memory engine: vector index warmed", "vectors", count)
	return nil
}

func (e *Engine) decayLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(e.cfg.DecayIntervalHours) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.decay.RunArchival(ctx); err != nil {
				slog.Warn("memory decay: archival error", "error", err)
			}
		}
	}
}

func lastUserMessage(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// LoadConfig reads ~/.ollama/server.json and merges the "memory" section.
// ---------------------------------------------------------------------------

type serverJSON struct {
	Memory Config `json:"memory"`
}

// LoadConfig reads the server config and returns a merged memory Config.
func LoadConfig() Config {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(home, ".ollama", "server.json"))
	if err != nil {
		return cfg
	}

	var sj serverJSON
	if err := json.Unmarshal(data, &sj); err != nil {
		slog.Warn("memory: failed to parse server.json", "error", err)
		return cfg
	}

	cfg.Merge(sj.Memory)
	return cfg
}
