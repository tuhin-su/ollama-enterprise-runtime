package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// OllamaEmbedder implements EmbeddingProvider by calling Ollama's own
// /api/embed endpoint, so no external embedding service is required.
type OllamaEmbedder struct {
	model      string
	baseURL    string
	client     *http.Client
	dimensions int
	mu         sync.RWMutex
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// NewOllamaEmbedder creates an embedder that calls the local Ollama server.
func NewOllamaEmbedder(model, baseURL string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	return &OllamaEmbedder{
		model:   model,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Embed returns the embedding for a single text.
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("memory embedder: empty response for text")
	}
	return embeddings[0], nil
}

// EmbedBatch returns embeddings for multiple texts in a single call.
func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody, err := json.Marshal(embedRequest{
		Model: e.model,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("memory embedder: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/api/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("memory embedder: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memory embedder: do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("memory embedder: status %d", resp.StatusCode)
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("memory embedder: decode: %w", err)
	}

	// Cache dimensions from first successful response
	if len(result.Embeddings) > 0 && len(result.Embeddings[0]) > 0 {
		e.mu.Lock()
		if e.dimensions == 0 {
			e.dimensions = len(result.Embeddings[0])
		}
		e.mu.Unlock()
	}

	return result.Embeddings, nil
}

// Dimensions returns the embedding dimensionality. Returns 0 until
// the first successful Embed call.
func (e *OllamaEmbedder) Dimensions() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dimensions
}
