package memory

import (
	"context"
	"log/slog"
	"math"
	"time"
)

// DecayProcessor handles time-based importance decay and archival of old
// memories. It is designed to run periodically via the WorkerPool.
type DecayProcessor struct {
	store    MemoryStore
	config   Config
	index    VectorIndex
	cache    CacheProvider
}

// NewDecayProcessor creates a decay processor.
func NewDecayProcessor(store MemoryStore, index VectorIndex, cache CacheProvider, cfg Config) *DecayProcessor {
	return &DecayProcessor{
		store:  store,
		config: cfg,
		index:  index,
		cache:  cache,
	}
}

// RunDecay applies importance decay to all non-pinned, non-archived memories
// for the given user. The decay formula is:
//
//	new_importance = importance * exp(-decay_rate * days_since_last_access)
//
// Memories whose importance drops below the importance threshold are archived.
func (d *DecayProcessor) RunDecay(ctx context.Context, userID string) error {
	memories, err := d.store.List(ctx, userID, ListOptions{
		Archived: boolPtr(false),
		Limit:    10000,
	})
	if err != nil {
		return err
	}

	now := time.Now()
	var decayed, archived int

	for _, mem := range memories {
		if mem.Pinned {
			continue
		}

		daysSinceAccess := now.Sub(mem.LastAccessed).Hours() / 24.0
		if daysSinceAccess < 1 {
			continue // too recent to decay
		}

		decayFactor := math.Exp(-d.config.DecayRate * daysSinceAccess)
		newImportance := mem.Importance * decayFactor

		// Clamp to minimum
		if newImportance < 0.01 {
			newImportance = 0.01
		}

		if newImportance != mem.Importance {
			mem.Importance = newImportance
			mem.UpdatedAt = now

			if newImportance < d.config.ImportanceThreshold {
				mem.Archived = true
				archived++

				// Remove from vector index — archived memories
				// are excluded from search by default.
				if d.index != nil {
					_ = d.index.Remove(mem.ID)
				}
			}

			if err := d.store.Update(ctx, mem); err != nil {
				slog.Warn("memory decay: failed to update", "id", mem.ID, "error", err)
				continue
			}

			// Invalidate cache
			if d.cache != nil {
				d.cache.Del(MemoryCacheKey(mem.ID))
			}

			decayed++
		}
	}

	slog.Info("memory decay complete",
		"user_id", userID,
		"processed", len(memories),
		"decayed", decayed,
		"archived", archived,
	)
	return nil
}

// RunArchival archives memories that haven't been accessed within
// ArchiveAfterDays regardless of their importance score.
func (d *DecayProcessor) RunArchival(ctx context.Context) error {
	cutoff := time.Now().AddDate(0, 0, -d.config.ArchiveAfterDays)
	count, err := d.store.ArchiveOlderThan(ctx, cutoff, d.config.ImportanceThreshold)
	if err != nil {
		return err
	}

	if count > 0 {
		slog.Info("memory archival complete", "archived", count, "cutoff", cutoff.Format(time.RFC3339))
		if d.cache != nil {
			d.cache.Clear()
		}
	}
	return nil
}

// BoostAccess increases a memory's importance when it is accessed,
// counteracting natural decay for frequently-used memories.
func BoostAccess(mem *Memory) {
	// Logarithmic boost: diminishing returns for very frequent access
	boost := 0.02 * math.Log1p(float64(mem.AccessCount))
	mem.Importance = math.Min(1.0, mem.Importance+boost)
	mem.LastAccessed = time.Now()
}

// MemoryCacheKey returns the cache key for a memory by ID.
func MemoryCacheKey(id string) string {
	return "mem:" + id
}

// SearchCacheKey returns the cache key for a search result set.
func SearchCacheKey(userID, query string) string {
	return "search:" + userID + ":" + query
}

// UserCacheKey returns the cache key for user-level data.
func UserCacheKey(userID string) string {
	return "user:" + userID
}

func boolPtr(b bool) *bool {
	return &b
}
