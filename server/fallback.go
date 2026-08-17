package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/loom/loom/api"
)

// FallbackStrategy defines how a failed operation (model inference, tool call, memory search) is recovered.
type FallbackStrategy string

const (
	FallbackStrategyModelSwap    FallbackStrategy = "model_swap"
	FallbackStrategyToolRetry    FallbackStrategy = "tool_retry"
	FallbackStrategyGracefulText FallbackStrategy = "graceful_text"
)

// ErrorLogRecord records an execution failure for audit and autonomous diagnosis.
type ErrorLogRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Component string    `json:"component"` // "inference", "tool", "memory", "chain"
	Target    string    `json:"target"`    // model name or tool name
	Error     string    `json:"error"`
	Recovered bool      `json:"recovered"`
	Fallback  string    `json:"fallback,omitempty"`
}

// FallbackManager handles intelligent error interception, automatic model swapping, and tool call retry.
type FallbackManager struct {
	mu            sync.RWMutex
	errorLogs     []ErrorLogRecord
	fallbackMap   map[string]string // Primary model -> Fallback model mapping
	maxLogEntries int
}

var (
	globalFallbackManager *FallbackManager
	onceFallbackManager   sync.Once
)

// GetFallbackManager returns the singleton FallbackManager instance.
func GetFallbackManager() *FallbackManager {
	onceFallbackManager.Do(func() {
		globalFallbackManager = &FallbackManager{
			errorLogs:     make([]ErrorLogRecord, 0, 100),
			fallbackMap:   make(map[string]string),
			maxLogEntries: 500,
		}
	})
	return globalFallbackManager
}

// RegisterFallback pairs a primary model or tool with a fallback candidate.
func (fm *FallbackManager) RegisterFallback(primary, fallback string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.fallbackMap[primary] = fallback
}

// RecordError logs an error event and tracks whether recovery succeeded.
func (fm *FallbackManager) RecordError(component, target string, err error, recovered bool, fallback string) {
	if err == nil {
		return
	}
	fm.mu.Lock()
	defer fm.mu.Unlock()

	rec := ErrorLogRecord{
		Timestamp: time.Now(),
		Component: component,
		Target:    target,
		Error:     err.Error(),
		Recovered: recovered,
		Fallback:  fallback,
	}

	if len(fm.errorLogs) >= fm.maxLogEntries {
		fm.errorLogs = fm.errorLogs[1:]
	}
	fm.errorLogs = append(fm.errorLogs, rec)

	slog.Warn("fallback manager: intercepted error",
		"component", component,
		"target", target,
		"error", err.Error(),
		"recovered", recovered,
		"fallback", fallback,
	)
}

// GetErrorLogs returns recent error records for diagnostic inspection.
func (fm *FallbackManager) GetErrorLogs() []ErrorLogRecord {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	logs := make([]ErrorLogRecord, len(fm.errorLogs))
	copy(logs, fm.errorLogs)
	return logs
}

// ExecuteWithModelFallback executes an inference function with a target model. If it fails, it automatically swaps to a fallback model.
func (fm *FallbackManager) ExecuteWithModelFallback(
	ctx context.Context,
	primaryModel string,
	s *Server,
	fn func(modelName string) (string, error),
) (string, string, error) {
	// Attempt 1: Primary Model
	output, err := fn(primaryModel)
	if err == nil {
		return output, primaryModel, nil
	}

	fm.RecordError("inference", primaryModel, err, false, "")

	// Resolve fallback model candidate
	fallbackModel := fm.resolveFallbackModel(primaryModel, s)
	if fallbackModel == "" || fallbackModel == primaryModel {
		return "", primaryModel, fmt.Errorf("primary model '%s' failed: %w (no alternative fallback model available)", primaryModel, err)
	}

	slog.Info("fallback manager: triggering model swap", "from", primaryModel, "to", fallbackModel)

	// Attempt 2: Fallback Model
	output, fbErr := fn(fallbackModel)
	if fbErr == nil {
		fm.RecordError("inference", primaryModel, err, true, fallbackModel)
		return output, fallbackModel, nil
	}

	fm.RecordError("inference", fallbackModel, fbErr, false, "")
	return "", primaryModel, fmt.Errorf("both primary '%s' (%v) and fallback '%s' (%v) failed", primaryModel, err, fallbackModel, fbErr)
}

// ExecuteWithToolFallback executes a tool function with retry and graceful error message recovery.
func (fm *FallbackManager) ExecuteWithToolFallback(
	ctx context.Context,
	toolName string,
	fn func() (map[string]any, error),
) map[string]any {
	// Attempt 1: Native tool call
	res, err := fn()
	if err == nil {
		return res
	}

	fm.RecordError("tool", toolName, err, false, "")

	// Retry once after brief pause
	time.Sleep(100 * time.Millisecond)
	res, retryErr := fn()
	if retryErr == nil {
		fm.RecordError("tool", toolName, err, true, "retry_success")
		return res
	}

	fm.RecordError("tool", toolName, retryErr, false, "graceful_error_payload")

	// Return graceful error payload so model flow does not crash
	return map[string]any{
		"error":           fmt.Sprintf("Tool '%s' execution encountered an error: %v", toolName, retryErr),
		"status":          "failed",
		"recovered_by":    "fallback_system",
		"recommendation": "Acknowledge the tool issue to the user and proceed with available text context.",
	}
}

func (fm *FallbackManager) resolveFallbackModel(primary string, s *Server) string {
	fm.mu.RLock()
	if fb, ok := fm.fallbackMap[primary]; ok && fb != "" {
		fm.mu.RUnlock()
		return fb
	}
	fm.mu.RUnlock()

	// Dynamic resolution: pick any other loaded/available model from local manifests
	models, err := s.listModels(context.Background())
	if err != nil || len(models) == 0 {
		return ""
	}

	for _, m := range models {
		if m.Name != primary {
			return m.Name
		}
	}

	return ""
}

// GetFallbackTools returns tool definitions to inspect fallback status and error logs.
func GetFallbackTools() api.Tools {
	logProps := api.NewToolPropertiesMap()

	return api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "get_error_logs",
				Description: "Retrieve recent runtime errors, tool failures, and automatic fallback recovery records.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: logProps,
				},
			},
		},
	}
}
