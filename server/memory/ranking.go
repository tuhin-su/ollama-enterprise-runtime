package memory

import (
	"math"
	"sort"
	"time"
)

// Ranker scores and sorts memories using a weighted combination of
// semantic similarity, importance, recency, access frequency, and pin status.
type Ranker struct {
	weights RankingWeights
}

// NewRanker creates a ranker with the given weights.
func NewRanker(weights RankingWeights) *Ranker {
	return &Ranker{weights: weights}
}

// Rank scores each memory and returns results sorted by score descending.
// similarities maps memory ID → cosine similarity from vector search.
func (r *Ranker) Rank(memories []*Memory, similarities map[string]float64) []*SearchResult {
	if len(memories) == 0 {
		return nil
	}

	results := make([]*SearchResult, 0, len(memories))

	for _, mem := range memories {
		sim := similarities[mem.ID]

		importanceScore := clamp01(mem.Importance)
		recencyScore := recencyDecay(time.Since(mem.LastAccessed))
		frequencyScore := clamp01(math.Log1p(float64(mem.AccessCount)) / math.Log1p(100))

		var pinnedBoost float64
		if mem.Pinned {
			pinnedBoost = 1.0
		}

		score := r.weights.Similarity*sim +
			r.weights.Importance*importanceScore +
			r.weights.Recency*recencyScore +
			r.weights.Frequency*frequencyScore +
			r.weights.Pinned*pinnedBoost

		results = append(results, &SearchResult{
			Memory:     mem,
			Score:      score,
			Similarity: sim,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// recencyDecay returns a score in [0, 1] that decays exponentially
// with time. Half-life is approximately 30 days.
func recencyDecay(age time.Duration) float64 {
	days := age.Hours() / 24.0
	if days < 0 {
		days = 0
	}
	return math.Exp(-days / 30.0)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
