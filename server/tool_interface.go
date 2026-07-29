package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ollama/ollama/api"
)

// VRAMArbiter arbitrates VRAM usage across concurrent request types (live chat, background task scheduler, and chaining)
var VRAMArbiter sync.RWMutex

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for the tool interface, or configure based on env
	},
}

// Global state for tool interface connections
type ToolServer struct {
	sync.RWMutex
	AuthToken            string
	ConnectedTools       map[string]*websocket.Conn
	ConnectedToolSchemas map[string]api.Tool
	PendingRequests      map[string]*websocket.Conn
	PendingHTTPRequests  map[string]chan map[string]interface{}
}

var globalToolServer = &ToolServer{
	AuthToken:            "abc", // Matched to ~/.ollama/server.json api_token
	ConnectedTools:       make(map[string]*websocket.Conn),
	ConnectedToolSchemas: make(map[string]api.Tool),
	PendingRequests:      make(map[string]*websocket.Conn),
	PendingHTTPRequests:  make(map[string]chan map[string]interface{}),
}

type AuthMessage struct {
	AuthToken  string   `json:"auth_token"`
	Role       string   `json:"role"`      // "model" or "tool"
	ToolName   string   `json:"tool_name"` // e.g. "calculator"
	ToolSchema api.Tool `json:"tool_schema,omitempty"` // The JSON schema of the tool
}

type BaseMessage struct {
	Type string `json:"type"`
}

type ModelCallToolMessage struct {
	Type       string                 `json:"type"` // "model_call_tool"
	RequestID  string                 `json:"request_id"`
	TargetTool string                 `json:"target_tool"`
	Payload    map[string]interface{} `json:"payload"`
}

type ToolCallModelMessage struct {
	Type       string                 `json:"type"` // "tool_call_model"
	RequestID  string                 `json:"request_id"`
	SourceTool string                 `json:"source_tool"`
	Payload    map[string]interface{} `json:"payload"`
}

func (s *Server) ToolInterfaceHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade websocket: %v\n", err)
		return
	}
	defer conn.Close()

	// 1. Authenticate
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Failed to read auth message: %v\n", err)
		return
	}

	var auth AuthMessage
	if err := json.Unmarshal(msg, &auth); err != nil || auth.AuthToken != globalToolServer.AuthToken {
		conn.WriteJSON(map[string]string{"status": "error", "message": "Invalid auth_token"})
		return
	}

	// 2. Register connection
	globalToolServer.Lock()
	if auth.Role == "model" {
		log.Println("Model connected to Tool Interface.")
	} else {
		if auth.ToolName == "" {
			auth.ToolName = "unknown"
		}
		globalToolServer.ConnectedTools[auth.ToolName] = conn
		if auth.ToolSchema.Type != "" {
			globalToolServer.ConnectedToolSchemas[auth.ToolName] = auth.ToolSchema
			if globalToolManager != nil {
				_ = globalToolManager.AddTool(c.Request.Context(), auth.ToolName, auth.ToolSchema)
			}
		}
		log.Printf("Tool '%s' connected to Tool Interface.\n", auth.ToolName)
	}
	globalToolServer.Unlock()

	conn.WriteJSON(map[string]string{"status": "authenticated"})

	// Reset read deadline for ongoing communication
	conn.SetReadDeadline(time.Time{})

	// 3. Message loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var base BaseMessage
		if err := json.Unmarshal(msg, &base); err != nil {
			continue
		}

		switch base.Type {
		case "model_call_tool":
			var m ModelCallToolMessage
			if err := json.Unmarshal(msg, &m); err == nil {
				if m.RequestID != "" {
					globalToolServer.Lock()
					globalToolServer.PendingRequests[m.RequestID] = conn
					globalToolServer.Unlock()
				}

				globalToolServer.RLock()
				toolConn, ok := globalToolServer.ConnectedTools[m.TargetTool]
				globalToolServer.RUnlock()

				if ok {
					toolConn.WriteJSON(map[string]interface{}{
						"type":       "execute_tool",
						"request_id": m.RequestID,
						"payload":    m.Payload,
					})
				} else {
					conn.WriteJSON(map[string]string{
						"type":       "error",
						"request_id": m.RequestID,
						"message":    "Tool '" + m.TargetTool + "' is not connected",
					})
				}
			}
		case "tool_call_model":
			var m ToolCallModelMessage
			if err := json.Unmarshal(msg, &m); err == nil {
				globalToolServer.Lock()
				modelConn := globalToolServer.PendingRequests[m.RequestID]
				delete(globalToolServer.PendingRequests, m.RequestID) // clean up
				
				httpCh, ok := globalToolServer.PendingHTTPRequests[m.RequestID]
				if ok {
					delete(globalToolServer.PendingHTTPRequests, m.RequestID)
				}
				globalToolServer.Unlock()

				if modelConn != nil {
					modelConn.WriteJSON(map[string]interface{}{
						"type":        "tool_event",
						"request_id":  m.RequestID,
						"source_tool": m.SourceTool,
						"payload":     m.Payload,
					})
				} else if ok && httpCh != nil {
					httpCh <- m.Payload
				}
			}
		}
	}

	// 4. Cleanup on disconnect
	globalToolServer.Lock()
	if auth.Role == "model" {
		for reqID, pendingConn := range globalToolServer.PendingRequests {
			if pendingConn == conn {
				delete(globalToolServer.PendingRequests, reqID)
			}
		}
		log.Println("Model disconnected.")
	} else if auth.Role == "tool" {
		delete(globalToolServer.ConnectedTools, auth.ToolName)
		delete(globalToolServer.ConnectedToolSchemas, auth.ToolName)
		if globalToolManager != nil {
			globalToolManager.RemoveTool(context.Background(), auth.ToolName)
		}
		log.Printf("Tool '%s' disconnected.\n", auth.ToolName)
	}
	globalToolServer.Unlock()
}

// GetActiveTools returns the list of ToolSchemas for all currently connected tools.
func (s *ToolServer) GetActiveTools() []api.Tool {
	s.RLock()
	defer s.RUnlock()
	var tools []api.Tool
	for _, schema := range s.ConnectedToolSchemas {
		tools = append(tools, schema)
	}
	return tools
}

// HasTool returns true if the tool is connected over WS.
func (s *ToolServer) HasTool(name string) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.ConnectedTools[name]
	return ok
}

// ExecuteTool synchronously sends an execution request to a connected WebSocket tool and awaits its response.
func (s *ToolServer) ExecuteTool(ctx context.Context, toolName string, payload map[string]interface{}) (map[string]interface{}, error) {
	s.RLock()
	conn, ok := s.ConnectedTools[toolName]
	s.RUnlock()

	if !ok {
		return nil, fmt.Errorf("tool '%s' is not connected", toolName)
	}

	// Generate a unique Request ID (simple timestamp + nanoseconds)
	reqID := fmt.Sprintf("req_%d_%d", time.Now().Unix(), time.Now().UnixNano())
	ch := make(chan map[string]interface{}, 1)

	s.Lock()
	s.PendingHTTPRequests[reqID] = ch
	s.Unlock()

	defer func() {
		s.Lock()
		delete(s.PendingHTTPRequests, reqID)
		s.Unlock()
	}()

	// Send execution request to WebSocket tool
	err := conn.WriteJSON(map[string]interface{}{
		"type":       "execute_tool",
		"request_id": reqID,
		"payload":    payload,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send request to tool: %w", err)
	}

	// Wait for response or timeout (15s)
	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(15 * time.Second):
		return nil, fmt.Errorf("tool execution timed out after 15 seconds")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
