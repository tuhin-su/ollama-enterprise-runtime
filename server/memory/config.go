package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config controls the memory subsystem. Loaded from the "memory" key in
// ~/.ollama/server.json.
type Config struct {
	// Enabled activates the memory middleware. Default false.
	Enabled bool `json:"enabled"`

	// DBPath is the SQLite database file.
	// Default: ~/.ollama/memory.db
	DBPath string `json:"db_path,omitempty"`

	// EmbeddingModel is the Ollama model used for embedding generation.
	// Default: "nomic-embed-text"
	EmbeddingModel string `json:"embedding_model,omitempty"`

	// TopK is the number of vector results to retrieve before ranking.
	// Default: 20
	TopK int `json:"top_k,omitempty"`

	// SimilarityThreshold is the minimum cosine similarity for inclusion.
	// Default: 0.65
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`

	// ImportanceThreshold is the minimum importance score for storing.
	// Default: 0.3
	ImportanceThreshold float64 `json:"importance_threshold,omitempty"`

	// DecayRate controls how fast importance fades per day.
	// Default: 0.01
	DecayRate float64 `json:"decay_rate,omitempty"`

	// CacheSize is the maximum number of items in the Ristretto cache.
	// Default: 10_000
	CacheSize int64 `json:"cache_size,omitempty"`

	// CacheMaxCost is the maximum total cost the cache will hold (bytes).
	// Default: 64 MiB
	CacheMaxCost int64 `json:"cache_max_cost,omitempty"`

	// CacheTTL is the default time-to-live for cached items.
	// Default: 5m
	CacheTTL Duration `json:"cache_ttl,omitempty"`

	// WorkerCount is the number of background goroutines.
	// Default: 4
	WorkerCount int `json:"worker_count,omitempty"`

	// MaxPromptMemories is the maximum number of memories injected into
	// the system prompt per request.
	// Default: 10
	MaxPromptMemories int `json:"max_prompt_memories,omitempty"`

	// MaxPromptTokens caps the token budget spent on memory context.
	// Default: 2048
	MaxPromptTokens int `json:"max_prompt_tokens,omitempty"`

	// DecayIntervalHours sets how often the decay worker runs.
	// Default: 24
	DecayIntervalHours int `json:"decay_interval_hours,omitempty"`

	// ArchiveAfterDays moves untouched memories to archive after N days.
	// Default: 90
	ArchiveAfterDays int `json:"archive_after_days,omitempty"`

	// RankingWeights controls the multi-signal ranking formula.
	Ranking RankingWeights `json:"ranking,omitempty"`
}

// RankingWeights allows tuning the relative importance of each signal.
type RankingWeights struct {
	Similarity float64 `json:"similarity,omitempty"` // default 0.4
	Importance float64 `json:"importance,omitempty"` // default 0.25
	Recency    float64 `json:"recency,omitempty"`    // default 0.2
	Frequency  float64 `json:"frequency,omitempty"`  // default 0.1
	Pinned     float64 `json:"pinned,omitempty"`     // default 0.05
}

// Duration is a JSON-friendly wrapper for time.Duration.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

// DefaultConfig returns a Config with production-ready defaults.
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".ollama", "memory.db")

	return Config{
		Enabled:             false,
		DBPath:              dbPath,
		EmbeddingModel:      "nomic-embed-text",
		TopK:                20,
		SimilarityThreshold: 0.65,
		ImportanceThreshold: 0.3,
		DecayRate:           0.01,
		CacheSize:           10_000,
		CacheMaxCost:        64 << 20, // 64 MiB
		CacheTTL:            Duration{5 * time.Minute},
		WorkerCount:         4,
		MaxPromptMemories:   10,
		MaxPromptTokens:     2048,
		DecayIntervalHours:  24,
		ArchiveAfterDays:    90,
		Ranking: RankingWeights{
			Similarity: 0.4,
			Importance: 0.25,
			Recency:    0.2,
			Frequency:  0.1,
			Pinned:     0.05,
		},
	}
}

// Merge applies non-zero overrides from other onto c.
func (c *Config) Merge(other Config) {
	if other.DBPath != "" {
		c.DBPath = other.DBPath
	}
	if other.EmbeddingModel != "" {
		c.EmbeddingModel = other.EmbeddingModel
	}
	if other.TopK > 0 {
		c.TopK = other.TopK
	}
	if other.SimilarityThreshold > 0 {
		c.SimilarityThreshold = other.SimilarityThreshold
	}
	if other.ImportanceThreshold > 0 {
		c.ImportanceThreshold = other.ImportanceThreshold
	}
	if other.DecayRate > 0 {
		c.DecayRate = other.DecayRate
	}
	if other.CacheSize > 0 {
		c.CacheSize = other.CacheSize
	}
	if other.CacheMaxCost > 0 {
		c.CacheMaxCost = other.CacheMaxCost
	}
	if other.CacheTTL.Duration > 0 {
		c.CacheTTL = other.CacheTTL
	}
	if other.WorkerCount > 0 {
		c.WorkerCount = other.WorkerCount
	}
	if other.MaxPromptMemories > 0 {
		c.MaxPromptMemories = other.MaxPromptMemories
	}
	if other.MaxPromptTokens > 0 {
		c.MaxPromptTokens = other.MaxPromptTokens
	}
	if other.DecayIntervalHours > 0 {
		c.DecayIntervalHours = other.DecayIntervalHours
	}
	if other.ArchiveAfterDays > 0 {
		c.ArchiveAfterDays = other.ArchiveAfterDays
	}
	if other.Ranking.Similarity > 0 {
		c.Ranking.Similarity = other.Ranking.Similarity
	}
	if other.Ranking.Importance > 0 {
		c.Ranking.Importance = other.Ranking.Importance
	}
	if other.Ranking.Recency > 0 {
		c.Ranking.Recency = other.Ranking.Recency
	}
	if other.Ranking.Frequency > 0 {
		c.Ranking.Frequency = other.Ranking.Frequency
	}
	if other.Ranking.Pinned > 0 {
		c.Ranking.Pinned = other.Ranking.Pinned
	}
	c.Enabled = other.Enabled
}
