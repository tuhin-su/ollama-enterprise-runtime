package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/loom/loom/server/memory"
)

// DataFlowEvent captures a real-time event in Loom for RabbitMQ streaming.
type DataFlowEvent struct {
	EventID   string         `json:"event_id"`
	Timestamp time.Time      `json:"timestamp"`
	EventType string         `json:"event_type"` // "prompt", "token_stream", "tool_call", "memory_rag", "fallback", "ssl_step"
	Model     string         `json:"model,omitempty"`
	UserID    string         `json:"user_id,omitempty"`
	Payload   map[string]any `json:"payload"`
}

// RabbitMQPublisher handles streaming data flow events to external RabbitMQ/AMQP visualization brokers.
type RabbitMQPublisher struct {
	mu         sync.RWMutex
	enabled    bool
	amqpURL    string
	exchange   string
	queueName  string
	httpClient *http.Client
}

var (
	globalRabbitMQPublisher *RabbitMQPublisher
	onceRabbitMQ            sync.Once
)

// GetRabbitMQPublisher returns the singleton RabbitMQPublisher instance.
func GetRabbitMQPublisher() *RabbitMQPublisher {
	onceRabbitMQ.Do(func() {
		cfg := memory.LoadConfig()
		amqpURL := cfg.RabbitMQURL
		enabled := cfg.RabbitMQEnabled || amqpURL != ""

		if amqpURL == "" {
			amqpURL = "http://localhost:15672/api/exchanges/%2F/amq.default/publish" // Default RabbitMQ HTTP Management API
		}

		globalRabbitMQPublisher = &RabbitMQPublisher{
			enabled:   enabled,
			amqpURL:   amqpURL,
			exchange:  "loom_dataflow",
			queueName: "loom_visualization_queue",
			httpClient: &http.Client{
				Timeout: 2 * time.Second,
			},
		}
	})
	return globalRabbitMQPublisher
}

// PublishEvent streams a data flow event asynchronously to RabbitMQ if enabled.
func (p *RabbitMQPublisher) PublishEvent(eventType string, model string, payload map[string]any) {
	p.mu.RLock()
	if !p.enabled {
		p.mu.RUnlock()
		return
	}
	p.mu.RUnlock()

	event := DataFlowEvent{
		EventID:   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Timestamp: time.Now(),
		EventType: eventType,
		Model:     model,
		Payload:   payload,
	}

	// Dispatch asynchronously so request paths are never blocked
	go p.sendToRabbitMQ(event)
}

func (p *RabbitMQPublisher) sendToRabbitMQ(event DataFlowEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Format payload for RabbitMQ Management API / WebHook Bridge
	reqBody := map[string]any{
		"properties":       map[string]any{"content_type": "application/json"},
		"routing_key":      "loom.dataflow",
		"payload":          string(data),
		"payload_encoding": "string",
	}

	reqBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), "POST", p.amqpURL, bytes.NewBuffer(reqBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		slog.Debug("rabbitmq publisher: post failed", "url", p.amqpURL, "error", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("rabbitmq publisher: streamed dataflow event", "event_type", event.EventType, "status", resp.Status)
}

// GetStatus returns the RabbitMQ publisher configuration status.
func (p *RabbitMQPublisher) GetStatus() map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return map[string]any{
		"enabled":    p.enabled,
		"amqp_url":   p.amqpURL,
		"exchange":   p.exchange,
		"routing_key": "loom.dataflow",
	}
}
