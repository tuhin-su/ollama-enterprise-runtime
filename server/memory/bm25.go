package memory

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// BM25Index implements an in-memory sparse BM25 search index for ultra-fast exact keyword lookups (<1ms).
type BM25Index struct {
	mu         sync.RWMutex
	docs       map[string]BM25Document
	docFreq    map[string]int
	totalLen   int64
	k1         float64
	b          float64
}

// BM25Document represents an indexed text item.
type BM25Document struct {
	ID        string            `json:"id"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	TermFreqs map[string]int    `json:"term_freqs"`
	Len       int               `json:"len"`
}

// BM25SearchResult represents a matched document with its BM25 sparse score.
type BM25SearchResult struct {
	ID       string            `json:"id"`
	Score    float64           `json:"score"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func NewBM25Index() *BM25Index {
	return &BM25Index{
		docs:    make(map[string]BM25Document),
		docFreq: make(map[string]int),
		k1:      1.2,
		b:       0.75,
	}
}

func tokenize(text string) []string {
	if len(text) == 0 {
		return nil
	}
	// Zero-copy string reference
	rawStr := StringZeroCopy(BytesZeroCopy(text))
	matches := tokenizeRegex.FindAllString(strings.ToLower(rawStr), -1)
	return matches
}

// IndexDocument adds or updates a document in the BM25 index.
func (b *BM25Index) IndexDocument(id, content string, metadata map[string]string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If updating, remove previous stats
	if old, exists := b.docs[id]; exists {
		b.totalLen -= int64(old.Len)
		for term := range old.TermFreqs {
			b.docFreq[term]--
			if b.docFreq[term] <= 0 {
				delete(b.docFreq, term)
			}
		}
	}

	terms := tokenize(content)
	tfMap := make(map[string]int)
	for _, t := range terms {
		tfMap[t]++
	}

	for term := range tfMap {
		b.docFreq[term]++
	}

	doc := BM25Document{
		ID:        id,
		Content:   content,
		Metadata:  metadata,
		TermFreqs: tfMap,
		Len:       len(terms),
	}

	b.docs[id] = doc
	b.totalLen += int64(len(terms))
}

// Search performs BM25 sparse keyword search.
func (b *BM25Index) Search(query string, topK int) []BM25SearchResult {
	b.mu.RLock()
	defer b.mu.RUnlock()

	queryTerms := tokenize(query)
	if len(queryTerms) == 0 || len(b.docs) == 0 {
		return nil
	}

	N := float64(len(b.docs))
	avgDL := float64(b.totalLen) / N

	scores := make(map[string]float64)

	for _, term := range queryTerms {
		df, exists := b.docFreq[term]
		if !exists {
			continue
		}

		// IDF formula with Laplace smoothing
		idf := math.Log(1.0 + (N - float64(df) + 0.5)/(float64(df)+0.5))

		for id, doc := range b.docs {
			tf, inDoc := doc.TermFreqs[term]
			if !inDoc {
				continue
			}

			tfFloat := float64(tf)
			numerator := tfFloat * (b.k1 + 1.0)
			denominator := tfFloat + b.k1*(1.0-b.b+b.b*(float64(doc.Len)/avgDL))
			score := idf * (numerator / denominator)

			scores[id] += score
		}
	}

	results := make([]BM25SearchResult, 0, len(scores))
	for id, score := range scores {
		doc := b.docs[id]
		results = append(results, BM25SearchResult{
			ID:       id,
			Score:    score,
			Content:  doc.Content,
			Metadata: doc.Metadata,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	return results
}

// ReciprocalRankFusion (RRF) merges dense vector scores and sparse BM25 scores.
func ReciprocalRankFusion(denseIDs []string, sparseIDs []string, k float64) []string {
	if k <= 0 {
		k = 60.0
	}

	rrfScores := make(map[string]float64)

	for rank, id := range denseIDs {
		rrfScores[id] += 1.0 / (k + float64(rank+1))
	}

	for rank, id := range sparseIDs {
		rrfScores[id] += 1.0 / (k + float64(rank+1))
	}

	type rankedItem struct {
		id    string
		score float64
	}

	items := make([]rankedItem, 0, len(rrfScores))
	for id, score := range rrfScores {
		items = append(items, rankedItem{id: id, score: score})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.id
	}

	return result
}
