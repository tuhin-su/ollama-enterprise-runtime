package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/server/memory"
)

// GetMemoryTools returns the list of memory-related tools.
func GetMemoryTools() api.Tools {
	// save_memory properties
	saveProps := api.NewToolPropertiesMap()
	saveProps.Set("content", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The clear, concise fact or information to remember (e.g., 'User prefers Python over Go').",
	})
	saveProps.Set("type", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The category of memory: 'user', 'project', 'conversation', 'episodic', or 'semantic'.",
		Enum:        []any{"user", "project", "conversation", "episodic", "semantic"},
	})
	saveProps.Set("tags", api.ToolProperty{
		Type:        api.PropertyType{"array"},
		Description: "Optional tags to group this memory.",
		Items: api.ToolProperty{
			Type: api.PropertyType{"string"},
		},
	})

	// list_memories properties
	listProps := api.NewToolPropertiesMap()
	listProps.Set("type", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "Optional filter by category.",
		Enum:        []any{"user", "project", "conversation", "episodic", "semantic"},
	})
	listProps.Set("pinned", api.ToolProperty{
		Type:        api.PropertyType{"boolean"},
		Description: "Optional filter by pinned status.",
	})
	listProps.Set("archived", api.ToolProperty{
		Type:        api.PropertyType{"boolean"},
		Description: "Optional filter by archived status.",
	})

	// delete_memory properties
	deleteProps := api.NewToolPropertiesMap()
	deleteProps.Set("id", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The unique ID of the memory to delete.",
	})

	// save_special_memory properties (AI managed special table)
	saveSpecialProps := api.NewToolPropertiesMap()
	saveSpecialProps.Set("key", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The key or topic for this special memory.",
	})
	saveSpecialProps.Set("value", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The data or value content for this special memory.",
	})

	// delete_special_memory properties
	deleteSpecialProps := api.NewToolPropertiesMap()
	deleteSpecialProps.Set("id", api.ToolProperty{
		Type:        api.PropertyType{"string"},
		Description: "The unique ID of the special memory to delete.",
	})

	memTools := api.Tools{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "save_memory",
				Description: "Save a new fact, context, or piece of information to long-term memory.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: saveProps,
					Required:   []string{"content"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "list_memories",
				Description: "Search, filter, or retrieve saved memories.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: listProps,
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "delete_memory",
				Description: "Delete a memory by its unique ID.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: deleteProps,
					Required:   []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "save_special_memory",
				Description: "Save a piece of key-value data to the AI's special memory table managed entirely by the AI.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: saveSpecialProps,
					Required:   []string{"key", "value"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "list_special_memories",
				Description: "List all items in the AI's special memory table.",
				Parameters: api.ToolFunctionParameters{
					Type: "object",
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "delete_special_memory",
				Description: "Delete an item from the AI's special memory table by ID.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: deleteSpecialProps,
					Required:   []string{"id"},
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "read_system_logs",
				Description: "Read the latest internal system logs of the Ollama server.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: func() *api.ToolPropertiesMap {
						m := api.NewToolPropertiesMap()
						m.Set("lines", api.ToolProperty{
							Type:        api.PropertyType{"integer"},
							Description: "Number of log lines to read from the end. Default is 50.",
						})
						m.Set("start_line", api.ToolProperty{
							Type:        api.PropertyType{"integer"},
							Description: "Optional starting line number to read (1-indexed).",
						})
						m.Set("end_line", api.ToolProperty{
							Type:        api.PropertyType{"integer"},
							Description: "Optional ending line number to read (1-indexed, inclusive).",
						})
						return m
					}(),
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "check_data_flow",
				Description: "Retrieve system status, data flow, memory cache metrics, and scheduler queue size.",
				Parameters: api.ToolFunctionParameters{
					Type: "object",
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "restart_server",
				Description: "Perform a graceful self-restart of the Ollama server process.",
				Parameters: api.ToolFunctionParameters{
					Type: "object",
				},
			},
		},
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "system_tool",
				Description: "Perform system-level management actions including self-restart, loading a new model, unloading the current model, and retrieving runtime error/exception stats.",
				Parameters: api.ToolFunctionParameters{
					Type:       "object",
					Properties: func() *api.ToolPropertiesMap {
						m := api.NewToolPropertiesMap()
						m.Set("action", api.ToolProperty{
							Type:        api.PropertyType{"string"},
							Description: "The system management action to perform: 'restart', 'load_model', 'unload_model', 'get_status', or 'list_models'.",
							Enum:        []any{"restart", "load_model", "unload_model", "get_status", "list_models"},
						})
						m.Set("model_name", api.ToolProperty{
							Type:        api.PropertyType{"string"},
							Description: "The name of the model to load or unload (required for load_model/unload_model actions).",
						})
						return m
					}(),
				},
			},
		},
	}

	// Append chain pipeline tools
	memTools = append(memTools, GetChainTools()...)
	return memTools
}

// ExecuteMemoryTool runs a memory tool against the LanceDB store and returns the result as string/JSON.
func (s *Server) ExecuteMemoryTool(ctx context.Context, userID string, toolCall api.ToolCall) (resultStr string, err error) {
	if memoryEngine == nil {
		return "", fmt.Errorf("memory engine not initialized")
	}

	args := toolCall.Function.Arguments.ToMap()
	store := memoryEngine.Store()

	// Log tool invocation details to system log file
	argsBytes, _ := json.Marshal(args)
	slog.Info("tool invocation",
		"tool", toolCall.Function.Name,
		"method", "ExecuteMemoryTool",
		"req_data_size", len(argsBytes),
	)

	// Log tool response details to system log file
	defer func() {
		if err == nil {
			slog.Info("tool response",
				"tool", toolCall.Function.Name,
				"resp_data_size", len(resultStr),
			)
		} else {
			slog.Error("tool execution error",
				"tool", toolCall.Function.Name,
				"error", err.Error(),
			)
		}
	}()

	switch toolCall.Function.Name {
	case "save_memory":
		content, ok := args["content"].(string)
		if !ok || content == "" {
			return "", fmt.Errorf("content is required")
		}
		memType := memory.MemoryTypeSemantic
		if t, ok := args["type"].(string); ok && t != "" {
			memType = memory.MemoryType(t)
		}
		var tags []string
		if tSlice, ok := args["tags"].([]any); ok {
			for _, tVal := range tSlice {
				if ts, ok := tVal.(string); ok {
					tags = append(tags, ts)
				}
			}
		} else if tSliceStr, ok := args["tags"].([]string); ok {
			tags = tSliceStr
		}

		emb, err := memoryEngine.Embed(ctx, content)
		if err != nil {
			return "", fmt.Errorf("failed to generate embedding: %w", err)
		}

		mem := &memory.Memory{
			UserID:     userID,
			Type:       memType,
			Content:    content,
			Embedding:  emb,
			Tags:       tags,
			Importance: 0.8,
		}

		if err := store.Save(ctx, mem); err != nil {
			return "", fmt.Errorf("failed to save memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     mem.ID,
			"msg":    "Memory successfully saved.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "list_memories":
		opts := memory.ListOptions{}
		if t, ok := args["type"].(string); ok && t != "" {
			opts.Type = memory.MemoryType(t)
		}
		if p, ok := args["pinned"].(bool); ok {
			opts.Pinned = &p
		}
		if a, ok := args["archived"].(bool); ok {
			opts.Archived = &a
		}

		memories, err := store.List(ctx, userID, opts)
		if err != nil {
			return "", fmt.Errorf("failed to list memories: %w", err)
		}

		type SimpleMem struct {
			ID      string            `json:"id"`
			Type    memory.MemoryType `json:"type"`
			Content string            `json:"content"`
			Tags    []string          `json:"tags,omitempty"`
		}
		simpleMems := make([]SimpleMem, len(memories))
		for i, m := range memories {
			simpleMems[i] = SimpleMem{
				ID:      m.ID,
				Type:    m.Type,
				Content: m.Content,
				Tags:    m.Tags,
			}
		}

		resBytes, _ := json.Marshal(simpleMems)
		return string(resBytes), nil

	case "delete_memory":
		id, ok := args["id"].(string)
		if !ok || id == "" {
			return "", fmt.Errorf("id is required")
		}

		if err := store.Delete(ctx, id); err != nil {
			return "", fmt.Errorf("failed to delete memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     id,
			"msg":    "Memory successfully deleted.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "save_special_memory":
		key, ok := args["key"].(string)
		if !ok || key == "" {
			return "", fmt.Errorf("key is required")
		}
		value, ok := args["value"].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("value is required")
		}

		// Generate embedding for the special memory value
		emb, err := memoryEngine.Embed(ctx, fmt.Sprintf("%s: %s", key, value))
		if err != nil {
			return "", fmt.Errorf("failed to generate embedding: %w", err)
		}

		mem := &memory.SpecialMemory{
			UserID:    userID,
			Key:       key,
			Value:     value,
			Embedding: emb,
		}

		if err := store.SaveSpecialMemory(ctx, mem); err != nil {
			return "", fmt.Errorf("failed to save special memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     mem.ID,
			"msg":    "Special memory successfully saved.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "list_special_memories":
		memories, err := store.ListSpecialMemories(ctx, userID)
		if err != nil {
			return "", fmt.Errorf("failed to list special memories: %w", err)
		}

		resBytes, _ := json.Marshal(memories)
		return string(resBytes), nil

	case "delete_special_memory":
		id, ok := args["id"].(string)
		if !ok || id == "" {
			return "", fmt.Errorf("id is required")
		}

		if err := store.DeleteSpecialMemory(ctx, id); err != nil {
			return "", fmt.Errorf("failed to delete special memory: %w", err)
		}

		resMap := map[string]interface{}{
			"status": "success",
			"id":     id,
			"msg":    "Special memory successfully deleted.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "read_system_logs":
		cfg := memory.LoadConfig()
		if cfg.LogPath == "" {
			return "", fmt.Errorf("log path not configured")
		}

		file, err := os.Open(cfg.LogPath)
		if err != nil {
			return "", fmt.Errorf("failed to open log file: %w", err)
		}
		defer file.Close()

		var allLines []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			allLines = append(allLines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("failed to read log file: %w", err)
		}

		var startLine, endLine int
		if val, ok := args["start_line"].(float64); ok {
			startLine = int(val)
		} else if val, ok := args["start_line"].(int); ok {
			startLine = val
		}
		if val, ok := args["end_line"].(float64); ok {
			endLine = int(val)
		} else if val, ok := args["end_line"].(int); ok {
			endLine = val
		}

		var resultLines []string
		totalLines := len(allLines)

		if startLine > 0 {
			if endLine <= 0 || endLine > totalLines {
				endLine = totalLines
			}
			if startLine > totalLines {
				startLine = totalLines + 1
			}
			if startLine <= endLine {
				resultLines = allLines[startLine-1 : endLine]
			}
		} else {
			limit := 50
			if val, ok := args["lines"].(float64); ok {
				limit = int(val)
			} else if val, ok := args["lines"].(int); ok {
				limit = val
			}
			if len(allLines) > limit {
				resultLines = allLines[len(allLines)-limit:]
			} else {
				resultLines = allLines
			}
		}

		resMap := map[string]interface{}{
			"status":      "success",
			"total_lines": totalLines,
			"start_line":  startLine,
			"end_line":    endLine,
			"logs":        resultLines,
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "check_data_flow":
		var totalMemories int64
		var dbPath string
		var embeddingModel string
		cfg := memory.LoadConfig()
		dbPath = cfg.DBPath
		embeddingModel = cfg.EmbeddingModel

		if memoryEngine != nil {
			mems, err := store.List(ctx, "", memory.ListOptions{Limit: 100_000})
			if err == nil {
				totalMemories = int64(len(mems))
			}
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		resMap := map[string]interface{}{
			"status": "success",
			"memory_subsystem": map[string]interface{}{
				"enabled":          memoryEngine != nil,
				"db_path":          dbPath,
				"embedding_model":  embeddingModel,
				"total_memories":   totalMemories,
			},
			"go_runtime": map[string]interface{}{
				"goroutines":    runtime.NumGoroutine(),
				"heap_alloc":    m.HeapAlloc,
				"sys_memory":    m.Sys,
				"num_gc_cycles": m.NumGC,
			},
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "restart_server":
		go func() {
			time.Sleep(1 * time.Second)
			slog.Info("Self-restart triggered by model tool call")
			execPath, err := os.Executable()
			if err != nil {
				slog.Error("failed to get executable path for restart", "error", err)
				os.Exit(1)
			}
			err = syscall.Exec(execPath, os.Args, os.Environ())
			if err != nil {
				slog.Error("failed to restart server via exec", "error", err)
				os.Exit(1)
			}
		}()
		resMap := map[string]interface{}{
			"status":  "success",
			"message": "Server self-restart initiated. Connection will reset shortly.",
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	case "system_tool":
		action, _ := args["action"].(string)
		if action == "" {
			return "", fmt.Errorf("action is required")
		}

		switch action {
		case "restart":
			go func() {
				time.Sleep(1 * time.Second)
				slog.Info("Self-restart triggered by system_tool action")
				execPath, err := os.Executable()
				if err != nil {
					slog.Error("failed to get executable path for restart", "error", err)
					os.Exit(1)
				}
				err = syscall.Exec(execPath, os.Args, os.Environ())
				if err != nil {
					slog.Error("failed to restart server via exec", "error", err)
					os.Exit(1)
				}
			}()
			resMap := map[string]interface{}{
				"status":  "success",
				"message": "Server self-restart initiated via system_tool.",
			}
			resBytes, _ := json.Marshal(resMap)
			return string(resBytes), nil

		case "load_model":
			modelName, _ := args["model_name"].(string)
			if modelName == "" {
				return "", fmt.Errorf("model_name is required for load_model action")
			}
			model, err := GetModel(modelName)
			if err != nil {
				return "", fmt.Errorf("failed to get model '%s': %w", modelName, err)
			}
			runnerCh, errCh := s.sched.getRunner(ctx, model, api.DefaultOptions(), nil, true, false, nil)
			select {
			case ref := <-runnerCh:
				ref.refMu.Lock()
				ref.refCount--
				if ref.refCount <= 0 {
					if ref.sessionDuration <= 0 {
						if ref.expireTimer != nil {
							ref.expireTimer.Stop()
							ref.expireTimer = nil
						}
						s.sched.expiredCh <- ref
					} else if ref.expireTimer == nil {
						ref.expireTimer = time.AfterFunc(ref.sessionDuration, func() {
							ref.refMu.Lock()
							defer ref.refMu.Unlock()
							if ref.expireTimer != nil {
								ref.expireTimer = nil
								s.sched.expiredCh <- ref
							}
						})
					}
				}
				ref.refMu.Unlock()
				resMap := map[string]interface{}{
					"status":  "success",
					"message": fmt.Sprintf("Model '%s' successfully loaded into runner cache.", modelName),
				}
				resBytes, _ := json.Marshal(resMap)
				return string(resBytes), nil
			case err := <-errCh:
				return "", fmt.Errorf("failed to load model: %w", err)
			case <-ctx.Done():
				return "", ctx.Err()
			}

		case "unload_model":
			modelName, _ := args["model_name"].(string)
			if modelName == "" {
				return "", fmt.Errorf("model_name is required for unload_model action")
			}
			model, err := GetModel(modelName)
			if err != nil {
				return "", fmt.Errorf("failed to get model '%s': %w", modelName, err)
			}
			s.sched.expireRunner(model)
			resMap := map[string]interface{}{
				"status":  "success",
				"message": fmt.Sprintf("Model '%s' successfully unloaded from memory.", modelName),
			}
			resBytes, _ := json.Marshal(resMap)
			return string(resBytes), nil

		case "get_status":
			var warnCount, errCount int
			cfg := memory.LoadConfig()
			if cfg.LogPath != "" {
				if file, err := os.Open(cfg.LogPath); err == nil {
					scanner := bufio.NewScanner(file)
					for scanner.Scan() {
						text := scanner.Text()
						if strings.Contains(text, "WARN") || strings.Contains(text, "warn") {
							warnCount++
						}
						if strings.Contains(text, "ERR") || strings.Contains(text, "err") || strings.Contains(text, "panic") {
							errCount++
						}
					}
					file.Close()
				}
			}
			s.sched.loadedMu.Lock()
			activeRunnersCount := len(s.sched.loaded)
			s.sched.loadedMu.Unlock()

			resMap := map[string]interface{}{
				"status": "success",
				"stats": map[string]interface{}{
					"warning_logs_count": warnCount,
					"error_logs_count":   errCount,
					"active_runners":     activeRunnersCount,
				},
			}
			resBytes, _ := json.Marshal(resMap)
			return string(resBytes), nil

		case "list_models":
			models, errList := s.modelCaches.modelList.List(ctx)
			if errList != nil {
				return "", fmt.Errorf("failed to list models: %w", errList)
			}
			var modelInfos []map[string]interface{}
			for _, mInfo := range models {
				modelDetails := map[string]interface{}{
					"name":           mInfo.Name,
					"size":           mInfo.Size,
					"modified_at":    mInfo.ModifiedAt,
					"format":         mInfo.Details.Format,
					"family":         mInfo.Details.Family,
					"parameter_size": mInfo.Details.ParameterSize,
				}
				if mObj, errObj := GetModel(mInfo.Name); errObj == nil {
					modelDetails["system_prompt"] = mObj.System
					modelDetails["template"] = mObj.Template
					modelDetails["abilities"] = map[string]interface{}{
						"tools_support":  mObj.Config.Parser != "",
						"parser":         mObj.Config.Parser,
						"project_memory": mObj.Config.RemoteModel != "",
					}
				}
				modelInfos = append(modelInfos, modelDetails)
			}
			resMap := map[string]interface{}{
				"status": "success",
				"models": modelInfos,
			}
			resBytes, _ := json.Marshal(resMap)
			return string(resBytes), nil

		default:
			return "", fmt.Errorf("invalid action: %s", action)
		}

	case "chain_request":
		cfg := memory.LoadConfig()
		if !cfg.ChainEnabled {
			return "", fmt.Errorf("model chaining is disabled in configuration")
		}

		// Get the default model name for the pipeline
		defaultModel := cfg.DefaultModel
		if defaultModel == "" {
			// Try to determine from available models
			models, errList := s.modelCaches.modelList.List(ctx)
			if errList == nil && len(models) > 0 {
				defaultModel = models[0].Name
			}
		}

		// Collect progress messages to return as part of the tool result
		var progressLog []string
		progressFn := func(msg string) {
			progressLog = append(progressLog, msg)
			slog.Info("chain pipeline progress", "message", msg)
		}

		result, err := s.ExecuteChainPipeline(ctx, toolCall, defaultModel, progressFn)
		if err != nil {
			return "", fmt.Errorf("chain pipeline failed: %w", err)
		}

		resMap := map[string]interface{}{
			"status":   "success",
			"progress": progressLog,
			"result":   result,
		}
		resBytes, _ := json.Marshal(resMap)
		return string(resBytes), nil

	default:
		return "", fmt.Errorf("unknown memory tool: %s", toolCall.Function.Name)
	}
}

// IsMemoryTool returns true if the tool name belongs to one of the memory tools.
func IsMemoryTool(name string) bool {
	return name == "save_memory" || name == "list_memories" || name == "delete_memory" ||
		name == "save_special_memory" || name == "list_special_memories" || name == "delete_special_memory" ||
		name == "read_system_logs" || name == "check_data_flow" || name == "restart_server" ||
		name == "system_tool" || name == "chain_request"
}
