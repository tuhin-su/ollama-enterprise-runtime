package memory

import (
	"math"
	"sort"
	"sync"
)

// BruteForceIndex is a pure-Go VectorIndex that computes exact cosine
// similarity against all stored vectors. It implements the VectorIndex
// interface and can be swapped for a usearch HNSW index later.
//
// All vectors are stored L2-normalised so search only needs dot products.
type BruteForceIndex struct {
	mu      sync.RWMutex
	vectors map[string][]float32
}

// NewBruteForceIndex creates an empty index.
func NewBruteForceIndex() *BruteForceIndex {
	return &BruteForceIndex{
		vectors: make(map[string][]float32),
	}
}

// Add inserts or replaces a vector. The input is copied and normalised.
func (idx *BruteForceIndex) Add(id string, embedding []float32) error {
	if len(embedding) == 0 {
		return nil
	}

	normed := make([]float32, len(embedding))
	copy(normed, embedding)
	normalize(normed)

	idx.mu.Lock()
	idx.vectors[id] = normed
	idx.mu.Unlock()
	return nil
}

// Remove deletes a vector by ID.
func (idx *BruteForceIndex) Remove(id string) error {
	idx.mu.Lock()
	delete(idx.vectors, id)
	idx.mu.Unlock()
	return nil
}

// Search returns the topK most similar vectors to query, sorted by
// similarity descending. The query is normalised internally.
func (idx *BruteForceIndex) Search(query []float32, topK int) ([]VectorResult, error) {
	if len(query) == 0 || topK <= 0 {
		return nil, nil
	}

	// Normalise query once
	q := make([]float32, len(query))
	copy(q, query)
	normalize(q)

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	type scored struct {
		id  string
		sim float64
	}

	results := make([]scored, 0, len(idx.vectors))
	for id, vec := range idx.vectors {
		if len(vec) != len(q) {
			continue
		}
		sim := dotProduct(q, vec)
		results = append(results, scored{id: id, sim: sim})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].sim > results[j].sim
	})

	if topK > len(results) {
		topK = len(results)
	}

	out := make([]VectorResult, topK)
	for i := 0; i < topK; i++ {
		out[i] = VectorResult{
			ID:         results[i].id,
			Similarity: results[i].sim,
		}
	}
	return out, nil
}

// Size returns the number of indexed vectors.
func (idx *BruteForceIndex) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.vectors)
}

// Close releases all stored vectors.
func (idx *BruteForceIndex) Close() error {
	idx.mu.Lock()
	idx.vectors = nil
	idx.mu.Unlock()
	return nil
}

// -- math helpers (zero-alloc hot path) --

// dotProduct computes the dot product of two equal-length vectors.
// When both inputs are L2-normalised this equals cosine similarity.
func dotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

// normalize performs in-place L2 normalisation.
func normalize(v []float32) {
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	if sumSq == 0 {
		return
	}
	norm := float64(1.0) / math.Sqrt(sumSq)
	for i := range v {
		v[i] = float32(float64(v[i]) * norm)
	}
}
