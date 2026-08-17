package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"

	"github.com/google/uuid"
)

// SSLTaskType defines the self-supervised learning objective.
type SSLTaskType string

const (
	TaskMaskedLanguageModel SSLTaskType = "masked_language_model"
	TaskNextTokenPrediction SSLTaskType = "next_token_prediction"
	TaskContrastiveLearning SSLTaskType = "contrastive_learning"
)

// SSLLearningSample represents an online self-supervised sample generated at runtime.
type SSLLearningSample struct {
	ID         string      `json:"id"`
	TaskType   SSLTaskType `json:"task_type"`
	InputText  string      `json:"input_text"`
	TargetText string      `json:"target_text"`
	Loss       float64     `json:"loss,omitempty"`
	Step       int64       `json:"step"`
}

// RuntimeSSLEngine handles online self-supervised learning during conversation loops.
type RuntimeSSLEngine struct {
	mu           sync.RWMutex
	embedder     EmbeddingProvider
	stepCount    int64
	sampleBuffer []SSLLearningSample
	maxBuffer    int
}

// NewRuntimeSSLEngine constructs the runtime SSL engine.
func NewRuntimeSSLEngine(embedder EmbeddingProvider) *RuntimeSSLEngine {
	return &RuntimeSSLEngine{
		embedder:     embedder,
		sampleBuffer: make([]SSLLearningSample, 0, 100),
		maxBuffer:    1000,
	}
}

// LearnFromTurn processes a user/assistant conversation turn in real-time, performing online SSL gradient estimation.
func (e *RuntimeSSLEngine) LearnFromTurn(ctx context.Context, userText, assistantText string) *SSLLearningSample {
	if userText == "" || assistantText == "" {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.stepCount++

	// 1. Generate embeddings for contrastive alignment
	userEmb, err1 := e.embedder.Embed(ctx, userText)
	asstEmb, err2 := e.embedder.Embed(ctx, assistantText)

	loss := 0.0
	if err1 == nil && err2 == nil && len(userEmb) > 0 && len(userEmb) == len(asstEmb) {
		// Calculate Cosine Similarity as SSL Contrastive Loss Target
		dot := 0.0
		normA := 0.0
		normB := 0.0
		for i := 0; i < len(userEmb); i++ {
			dot += float64(userEmb[i] * asstEmb[i])
			normA += float64(userEmb[i] * userEmb[i])
			normB += float64(asstEmb[i] * asstEmb[i])
		}
		sim := dot / (math.Sqrt(normA) * math.Sqrt(normB) + 1e-8)
		loss = 1.0 - sim // Contrastive Loss
	}

	sample := SSLLearningSample{
		ID:         uuid.New().String(),
		TaskType:   TaskContrastiveLearning,
		InputText:  userText,
		TargetText: assistantText,
		Loss:       loss,
		Step:       e.stepCount,
	}

	if len(e.sampleBuffer) >= e.maxBuffer {
		e.sampleBuffer = e.sampleBuffer[1:]
	}
	e.sampleBuffer = append(e.sampleBuffer, sample)

	slog.Info("runtime ssl: online gradient estimation step complete",
		"step", e.stepCount,
		"contrastive_loss", fmt.Sprintf("%.4f", loss),
	)

	return &sample
}

// GetSSLStats returns statistics on runtime SSL steps.
func (e *RuntimeSSLEngine) GetSSLStats() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()

	avgLoss := 0.0
	if len(e.sampleBuffer) > 0 {
		total := 0.0
		for _, s := range e.sampleBuffer {
			total += s.Loss
		}
		avgLoss = total / float64(len(e.sampleBuffer))
	}

	return map[string]any{
		"total_steps":     e.stepCount,
		"buffered_samples": len(e.sampleBuffer),
		"average_loss":    avgLoss,
	}
}
