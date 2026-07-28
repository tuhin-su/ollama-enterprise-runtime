package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/llm"
	"github.com/ollama/ollama/server/memory"
	"github.com/ollama/ollama/types/model"
)

// ────────────────────────────────────────────────────────────────────────────
// Model Chaining / Pipeline Orchestrator
//
// When the default model determines it cannot handle a request (e.g. vision,
// code, math, multilingual) it delegates to a pipeline of specialist models
// that are selected dynamically from the locally available pool.
//
// Pipeline flow:
//   1. Default model receives user prompt
//   2. Default model calls `chain_request` tool with analysis of what's needed
//   3. Orchestrator builds the pipeline: selects models → chains them
//   4. Each step: unload previous → load next → run prompt → store result
//   5. Final aggregated result returned to default model for synthesis
//   6. User sees transparent progress at every step
// ────────────────────────────────────────────────────────────────────────────

// ChainStep represents one step in a model chain pipeline.
type ChainStep struct {
	StepIndex   int    `json:"step_index"`
	ModelName   string `json:"model_name"`
	Prompt      string `json:"prompt"`
	Output      string `json:"output,omitempty"`
	Status      string `json:"status"` // "pending", "running", "done", "error"
	Error       string `json:"error,omitempty"`
	DurationMs  int64  `json:"duration_ms,omitempty"`
	InputSize   int    `json:"input_size"`
	OutputSize  int    `json:"output_size,omitempty"`
}

// ChainPipeline holds the full pipeline state.
type ChainPipeline struct {
	ID              string      `json:"id"`
	OriginalPrompt  string      `json:"original_prompt"`
	DefaultModel    string      `json:"default_model"`
	Steps           []ChainStep `json:"steps"`
	FinalResult     string      `json:"final_result,omitempty"`
	Status          string      `json:"status"` // "planning", "running", "synthesizing", "done", "error"
	CreatedAt       time.Time   `json:"created_at"`
	TotalDurationMs int64       `json:"total_duration_ms,omitempty"`
}

// ChainRequest is the input the default model sends when invoking `chain_request`.
type ChainRequest struct {
	// Reason explains why the default model cannot handle this alone.
	Reason string `json:"reason"`
	// RequiredCapabilities lists what's needed (e.g. ["vision", "code", "math"]).
	RequiredCapabilities []string `json:"required_capabilities"`
	// SubTasks breaks the request into discrete tasks for different models.
	SubTasks []ChainSubTask `json:"sub_tasks"`
}

// ChainSubTask describes one piece of work in the chain.
type ChainSubTask struct {
	// Description of what this subtask needs to accomplish.
	Description string `json:"description"`
	// Prompt to send to the selected model.
	Prompt string `json:"prompt"`
	// RequiredCapability is the primary capability needed (e.g. "vision", "code").
	RequiredCapability string `json:"required_capability,omitempty"`
	// PreferredModel lets the AI suggest a specific model (optional).
	PreferredModel string `json:"preferred_model,omitempty"`
	// NeedsPreviousOutput if true, the previous step's output is prepended as context.
	NeedsPreviousOutput bool `json:"needs_previous_output"`
}

// GetChainTools returns the chain_request tool definition for injection into model tools.
func GetChainTools() api.Tools {
	subTaskProps := api.NewToolPropertiesMap()
	subTaskProps.Set("description", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Description of what this subtask accomplishes.",
	})
	subTaskProps.Set("prompt", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The prompt to send to the specialist model for this subtask.",
	})
	subTaskProps.Set("required_capability", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The primary capability needed: 'vision', 'code', 'math', 'tools', 'thinking', 'embedding', 'audio', 'image', or 'general'.",
	})
	subTaskProps.Set("preferred_model", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Optional: specific model name to use (from list_models results).",
	})
	subTaskProps.Set("needs_previous_output", api.ToolProperty{
		Type:        api.PropertyType{"boolean"},
		Description: "If true, the previous step's output will be included as context for this step.",
	})

	chainProps := api.NewToolPropertiesMap()
	chainProps.Set("reason", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Explain why you cannot handle this request alone and need to delegate to specialist models.",
	})
	chainProps.Set("required_capabilities", api.ToolProperty{
		Type:        api.PropertyType{"array"},
		Description: "List of capabilities needed to fulfill this request (e.g. ['vision', 'code']).",
		Items: api.ToolProperty{
			Type: api.PropertyType{"string"},
		},
	})
	chainProps.Set("sub_tasks", api.ToolProperty{
		Type:        api.PropertyType{"array"},
		Description: "Ordered list of subtasks to execute across specialist models. Each subtask is run sequentially and can reference previous outputs.",
		Items: api.ToolProperty{
			Type: api.PropertyType{"object"},
		},
	})

	return api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "chain_request",
				Description: "Delegate a complex request to a chain of specialist models when you cannot handle it alone. Analyze what capabilities are needed, break the work into subtasks, and the system will automatically select and orchestrate the best available models. Each step's output feeds into the next. The final aggregated result will be returned to you for synthesis.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: chainProps,
					Required:   []string{"reason", "sub_tasks"},
				},
			},
		},
	}
}

// IsChainTool returns true if the tool name is the chain orchestrator tool.
func IsChainTool(name string) bool {
	return name == "chain_request"
}

// ────────────────────────────────────────────────────────────────────────────
// Pipeline Execution Engine
// ────────────────────────────────────────────────────────────────────────────

// ExecuteChainPipeline runs the full chain pipeline on the server.
// Flow:
//  1. Unload the default model to free VRAM
//  2. For each sub-task: load specialist → run → unload specialist
//  3. Build aggregated synthesis string
//  4. Return to default model which writes the final answer
//
// Every state transition is emitted through progressFn so the user sees
// exactly what is happening at all times.
func (s *Server) ExecuteChainPipeline(
	ctx context.Context,
	toolCall api.ToolCall,
	defaultModelName string,
	progressFn func(msg string),
	images []api.ImageData,
) (string, error) {

	args := toolCall.Function.Arguments.ToMap()

	// Parse the chain request
	chainReq, err := parseChainRequest(args)
	if err != nil {
		return "", fmt.Errorf("invalid chain_request: %w", err)
	}

	cfg := memory.LoadConfig()
	maxSteps := cfg.ChainMaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}

	if len(chainReq.SubTasks) > maxSteps {
		return "", fmt.Errorf("chain pipeline has %d steps, maximum allowed is %d", len(chainReq.SubTasks), maxSteps)
	}

	pipeline := &ChainPipeline{
		ID:             fmt.Sprintf("chain-%d", time.Now().UnixNano()),
		OriginalPrompt: chainReq.Reason,
		DefaultModel:   defaultModelName,
		Status:         "planning",
		CreatedAt:      time.Now(),
	}

	slog.Info("chain pipeline started",
		"pipeline_id", pipeline.ID,
		"default_model", defaultModelName,
		"sub_tasks", len(chainReq.SubTasks),
		"reason", chainReq.Reason,
	)

	// ─── PHASE 1: Plan — resolve model for each subtask ───────────────────────
	progressFn(fmt.Sprintf(
		"\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
			"🔗 **Model Chain Pipeline** `%s`\n"+
			"📋 Reason: %s\n"+
			"📊 Total steps: %d\n"+
			"🤖 Default model: `%s`\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		pipeline.ID, chainReq.Reason, len(chainReq.SubTasks), defaultModelName))

	availableModels, err := s.getAvailableModelsWithCaps(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list available models: %w", err)
	}

	progressFn("\n📋 **Pipeline Plan:**")
	for i, task := range chainReq.SubTasks {
		prompt := task.Prompt
		if prompt == "" {
			prompt = task.Description
			if prompt == "" {
				prompt = "Please analyze this input."
			}
		}
		
		cap := task.RequiredCapability
		if cap == "" {
			// Auto-detect if they forgot to set required_capability
			if strings.Contains(strings.ToLower(task.Description), "image") || strings.Contains(strings.ToLower(task.Description), "vision") {
				cap = "vision"
				task.RequiredCapability = "vision"
			} else {
				cap = "general"
			}
		}

		selectedModel := s.selectModelForTask(task, availableModels, defaultModelName)
		pipeline.Steps = append(pipeline.Steps, ChainStep{
			StepIndex: i + 1,
			ModelName: selectedModel,
			Prompt:    prompt,
			Status:    "pending",
			InputSize: len(prompt),
		})
		progressFn(fmt.Sprintf("  %d. `%s` → **%s** [%s]", i+1, selectedModel, task.Description, cap))
	}

	// ─── PHASE 2: Unload default model to free VRAM ───────────────────────────
	// The default model called us via tool — it holds VRAM. We unload it so the
	// specialist models can load. It will be reloaded automatically when the
	// tool result is returned and the default model resumes.
	progressFn(fmt.Sprintf("\n🔻 **Unloading default model** `%s` to free memory for pipeline...", defaultModelName))
	s.unloadChainModelAndWait(defaultModelName)
	progressFn("  ✓ Default model unloaded.")

	// ─── PHASE 3: Execute each step sequentially ──────────────────────────────
	pipeline.Status = "running"
	progressFn("\n⚡ **Executing pipeline steps:**")

	startTime := time.Now()
	var previousOutput string

	for i := range pipeline.Steps {
		step := &pipeline.Steps[i]

		if ctx.Err() != nil {
			step.Status = "error"
			step.Error = "context cancelled"
			pipeline.Status = "error"
			return "", ctx.Err()
		}

		step.Status = "running"
		stepStart := time.Now()

		// Build prompt — prepend previous step output if requested
		fullPrompt := step.Prompt
		if i < len(chainReq.SubTasks) && chainReq.SubTasks[i].NeedsPreviousOutput && previousOutput != "" {
			fullPrompt = fmt.Sprintf(
				"## Output from previous step:\n%s\n\n## Your task:\n%s",
				previousOutput, step.Prompt,
			)
			step.InputSize = len(fullPrompt)
		}

		progressFn(fmt.Sprintf(
			"\n  ┌─ Step %d/%d ─────────────────────────────\n"+
				"  │ Model : `%s`\n"+
				"  │ Task  : %s\n"+
				"  │ Input : %d chars\n"+
				"  │ Status: ⏳ loading & running...",
			step.StepIndex, len(pipeline.Steps),
			step.ModelName, chainReq.SubTasks[i].Description, step.InputSize))

		output, execErr := s.executeChainStep(ctx, step.ModelName, fullPrompt, defaultModelName, images)
		step.DurationMs = time.Since(stepStart).Milliseconds()

		if execErr != nil {
			step.Status = "error"
			step.Error = execErr.Error()
			slog.Error("chain step failed",
				"pipeline_id", pipeline.ID,
				"step", step.StepIndex,
				"model", step.ModelName,
				"error", execErr,
			)
			progressFn(fmt.Sprintf(
				"  │ Status: ❌ FAILED — %s\n"+
					"  └─ Retrying with default model `%s`...",
				execErr.Error(), defaultModelName))

			// Fallback to default model for this step
			output, execErr = s.executeChainStep(ctx, defaultModelName, fullPrompt, defaultModelName, images)
			if execErr != nil {
				progressFn(fmt.Sprintf("  └─ ❌ Fallback also failed: %s", execErr.Error()))
				continue
			}
			step.ModelName = defaultModelName + " (fallback)"
			step.Status = "done"
		} else {
			step.Status = "done"
		}

		step.Output = output
		step.OutputSize = len(output)
		previousOutput = output

		progressFn(fmt.Sprintf(
			"  │ Status: ✅ done in %.1fs, output %d chars\n"+
				"  └───────────────────────────────────────",
			float64(step.DurationMs)/1000, step.OutputSize))

		// Unload the specialist model after use (skip default model / fallback)
		rawModel := step.ModelName
		if !strings.HasSuffix(rawModel, "(fallback)") && rawModel != defaultModelName {
			progressFn(fmt.Sprintf("  🔻 Unloading `%s` to free memory...", rawModel))
			s.unloadChainModelAndWait(rawModel)
			progressFn("  ✓ Unloaded.")
		}
	}

	pipeline.TotalDurationMs = time.Since(startTime).Milliseconds()

	// ─── PHASE 4: Synthesize ──────────────────────────────────────────────────
	pipeline.Status = "synthesizing"
	progressFn(fmt.Sprintf(
		"\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
			"🧩 Pipeline complete in %.1fs — reloading `%s` for final answer...\n"+
			"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		float64(pipeline.TotalDurationMs)/1000, defaultModelName))

	// Build the aggregated result returned to the default model as tool output
	var synthesis strings.Builder
	synthesis.WriteString("## Chain Pipeline Results\n\n")
	synthesis.WriteString(fmt.Sprintf("**Pipeline ID:** %s  |  **Total time:** %.1fs\n",
		pipeline.ID, float64(pipeline.TotalDurationMs)/1000))
	synthesis.WriteString(fmt.Sprintf("**Reason:** %s\n\n", chainReq.Reason))

	for _, step := range pipeline.Steps {
		synthesis.WriteString(fmt.Sprintf(
			"### Step %d — `%s` (%.1fs)\n",
			step.StepIndex, step.ModelName, float64(step.DurationMs)/1000))
		if step.Status == "done" && step.Output != "" {
			synthesis.WriteString(step.Output)
			synthesis.WriteString("\n\n")
		} else if step.Error != "" {
			synthesis.WriteString(fmt.Sprintf("⚠️ Error: %s\n\n", step.Error))
		} else {
			synthesis.WriteString("_(no output)_\n\n")
		}
	}

	pipeline.FinalResult = synthesis.String()
	pipeline.Status = "done"

	slog.Info("chain pipeline completed",
		"pipeline_id", pipeline.ID,
		"total_steps", len(pipeline.Steps),
		"total_duration_ms", pipeline.TotalDurationMs,
		"result_size", len(pipeline.FinalResult),
	)

	return pipeline.FinalResult, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ────────────────────────────────────────────────────────────────────────────

// modelCapInfo holds model metadata with parsed capabilities.
type modelCapInfo struct {
	Name         string
	Family       string
	ParamSize    string
	System       string
	Capabilities []model.Capability
	HasTools     bool
}

// getAvailableModelsWithCaps lists all local models with their capability info.
func (s *Server) getAvailableModelsWithCaps(ctx context.Context) ([]modelCapInfo, error) {
	models, err := s.modelCaches.modelList.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []modelCapInfo
	for _, mInfo := range models {
		info := modelCapInfo{
			Name:      mInfo.Name,
			Family:    mInfo.Details.Family,
			ParamSize: mInfo.Details.ParameterSize,
		}

		if mObj, errObj := GetModel(mInfo.Name); errObj == nil {
			info.System = mObj.System
			info.Capabilities = mObj.Capabilities()
			info.HasTools = mObj.Config.Parser != ""
		}

		result = append(result, info)
	}
	return result, nil
}

// selectModelForTask picks the best available model for a subtask.
func (s *Server) selectModelForTask(task ChainSubTask, available []modelCapInfo, defaultModel string) string {
	// If a preferred model was specified and it exists, use it
	if task.PreferredModel != "" {
		for _, m := range available {
			if m.Name == task.PreferredModel {
				return m.Name
			}
		}
	}

	// Map the required_capability string to model.Capability
	capMap := map[string]model.Capability{
		"vision":    model.CapabilityVision,
		"code":      model.CapabilityCompletion,
		"math":      model.CapabilityCompletion,
		"tools":     model.CapabilityTools,
		"thinking":  model.CapabilityThinking,
		"embedding": model.CapabilityEmbedding,
		"image":     model.CapabilityImage,
		"audio":     model.CapabilityAudio,
	}

	// Try to find a model with the required capability
	if reqCap, ok := capMap[task.RequiredCapability]; ok {
		// Priority: models that have the capability AND are not the default
		var candidates []modelCapInfo
		for _, m := range available {
			if m.Name == defaultModel {
				continue
			}
			for _, cap := range m.Capabilities {
				if cap == reqCap {
					candidates = append(candidates, m)
					break
				}
			}
		}
		if len(candidates) > 0 {
			return candidates[0].Name
		}
	}

	// Try matching by family or keyword in system prompt
	keyword := strings.ToLower(task.RequiredCapability)
	if keyword == "" {
		keyword = strings.ToLower(task.Description)
	}
	for _, m := range available {
		if m.Name == defaultModel {
			continue
		}
		nameLower := strings.ToLower(m.Name)
		familyLower := strings.ToLower(m.Family)
		systemLower := strings.ToLower(m.System)

		if strings.Contains(nameLower, keyword) ||
			strings.Contains(familyLower, keyword) ||
			strings.Contains(systemLower, keyword) {
			return m.Name
		}

		// Heuristic: code-related tasks match "code", "coder", "starcoder", "deepseek-coder" etc.
		if (keyword == "code" || keyword == "coding") &&
			(strings.Contains(nameLower, "code") || strings.Contains(nameLower, "coder") || strings.Contains(nameLower, "deepseek")) {
			return m.Name
		}
	}

	// Fallback: use the default model itself
	return defaultModel
}

// executeChainStep runs a single inference step against a specific model
// using the scheduler internally, similar to how the chat handler works.
func (s *Server) executeChainStep(ctx context.Context, modelName string, prompt string, defaultModelName string, images []api.ImageData) (string, error) {
	// Resolve the model
	mName := model.ParseName(modelName)
	mName, err := getExistingName(mName)
	if err != nil {
		return "", fmt.Errorf("model '%s' not found: %w", modelName, err)
	}

	m, err := GetModel(mName.String())
	if err != nil {
		return "", fmt.Errorf("failed to load model '%s': %w", modelName, err)
	}

	caps := []model.Capability{model.CapabilityCompletion}
	r, m, opts, err := s.scheduleRunner(ctx, mName.String(), caps, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to schedule runner for '%s': %w", modelName, err)
	}

	// Build messages for this step
	msgs := []api.Message{
		{Role: "system", Content: "You are a specialist model in a processing pipeline. Answer the following task precisely and concisely. Your output will be used as input for the next processing step or as part of a final aggregated answer."},
		{Role: "user", Content: prompt, Images: images},
	}

	if m.System != "" {
		msgs[0].Content = m.System + "\n\n" + msgs[0].Content
	}

	// Build the prompt from template
	promptOpts := optionsForPrompt(opts, r)
	templatePrompt, mediaData, err := chatPrompt(ctx, m, r.Tokenize, promptOpts, msgs, nil, nil, true)
	if err != nil {
		return "", fmt.Errorf("failed to build prompt for '%s': %w", modelName, err)
	}

	// Run completion and collect full output
	var output strings.Builder
	completionErr := r.Completion(ctx, llm.CompletionRequest{
		Prompt:   templatePrompt,
		Media:    mediaData,
		Options:  opts,
		Shift:    true,
		Truncate: true,
	}, func(resp llm.CompletionResponse) {
		output.WriteString(resp.Content)
	})

	if completionErr != nil {
		return "", fmt.Errorf("completion failed for '%s': %w", modelName, completionErr)
	}

	return output.String(), nil
}

// unloadChainModel triggers unloading of a model after a chain step to free memory.
// This is the fire-and-forget variant; prefer unloadChainModelAndWait for the pipeline.
func (s *Server) unloadChainModel(modelName string) {
	s.unloadChainModelAndWait(modelName)
}

// unloadChainModelAndWait expires the runner for modelName and then polls until
// it is actually removed from the scheduler's loaded map (or times out after 30s).
// This ensures the VRAM is freed before the next specialist model loads.
func (s *Server) unloadChainModelAndWait(modelName string) {
	// Strip "(fallback)" suffix if present
	cleanName := strings.TrimSuffix(strings.TrimSpace(modelName), " (fallback)")

	mName := model.ParseName(cleanName)
	mName, err := getExistingName(mName)
	if err != nil {
		slog.Warn("chain: model not found for unload", "model", cleanName)
		return
	}
	m, err := GetModel(mName.String())
	if err != nil {
		slog.Warn("chain: GetModel failed for unload", "model", cleanName, "error", err)
		return
	}

	// Signal the scheduler to expire this runner immediately
	s.sched.expireRunner(m)

	// Poll until the runner is gone from the loaded map (max 30s)
	deadline := time.Now().Add(30 * time.Second)
	modelKey := schedulerModelKey(m)
	for time.Now().Before(deadline) {
		s.sched.loadedMu.Lock()
		_, stillLoaded := s.sched.loaded[modelKey]
		s.sched.loadedMu.Unlock()
		if !stillLoaded {
			slog.Info("chain: model unloaded", "model", cleanName)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	slog.Warn("chain: timed out waiting for model to unload", "model", cleanName)
}

// parseChainRequest parses the raw tool call arguments into a ChainRequest.
func parseChainRequest(args map[string]any) (*ChainRequest, error) {
	req := &ChainRequest{}

	if reason, ok := args["reason"].(string); ok {
		req.Reason = reason
	}

	if caps, ok := args["required_capabilities"].([]any); ok {
		for _, c := range caps {
			if cs, ok := c.(string); ok {
				req.RequiredCapabilities = append(req.RequiredCapabilities, cs)
			}
		}
	}

	if tasks, ok := args["sub_tasks"].([]any); ok {
		for _, t := range tasks {
			taskMap, ok := t.(map[string]any)
			if !ok {
				continue
			}
			subTask := ChainSubTask{}
			if d, ok := taskMap["description"].(string); ok {
				subTask.Description = d
			}
			if p, ok := taskMap["prompt"].(string); ok {
				subTask.Prompt = p
			}
			if rc, ok := taskMap["required_capability"].(string); ok {
				subTask.RequiredCapability = rc
			}
			if pm, ok := taskMap["preferred_model"].(string); ok {
				subTask.PreferredModel = pm
			}
			if npo, ok := taskMap["needs_previous_output"].(bool); ok {
				subTask.NeedsPreviousOutput = npo
			}
			req.SubTasks = append(req.SubTasks, subTask)
		}
	}

	if len(req.SubTasks) == 0 {
		return nil, fmt.Errorf("chain_request requires at least one sub_task")
	}

	return req, nil
}

// ChainPipelineResultToJSON serializes the result for tool response.
func ChainPipelineResultToJSON(result string, pipeline *ChainPipeline) string {
	resMap := map[string]interface{}{
		"status":          "success",
		"pipeline_result": result,
	}
	if pipeline != nil {
		resMap["pipeline_id"] = pipeline.ID
		resMap["total_steps"] = len(pipeline.Steps)
		resMap["total_duration_ms"] = pipeline.TotalDurationMs
	}
	resBytes, _ := json.Marshal(resMap)
	return string(resBytes)
}

// ExecuteChainPipelineStreaming is the routes-facing entry point for chain_request
// that streams progress messages via the provided callback (e.g. to the HTTP response channel).
// It is identical to ExecuteChainPipeline but named explicitly so routes.go can distinguish
// chain tool execution from regular memory tool execution.
func (s *Server) ExecuteChainPipelineStreaming(
	ctx context.Context,
	toolCall api.ToolCall,
	defaultModelName string,
	streamProgress func(msg string),
	images []api.ImageData,
) (string, error) {
	return s.ExecuteChainPipeline(ctx, toolCall, defaultModelName, streamProgress, images)
}
