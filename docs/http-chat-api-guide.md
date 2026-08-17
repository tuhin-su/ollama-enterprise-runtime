# HTTP Chat Request & Ingestion API Documentation

This guide provides technical specifications and code examples for sending **HTTP Chat Requests**, **Token Streaming**, **Image Multi-Modal Payloads**, **Inline Tool Invocations**, and **Document Ingestion** using **Loom Enterprise Runtime** endpoints.

---

## 1. Native HTTP Chat Endpoint (`POST /api/chat`)

The native Loom chat endpoint supports structured conversation turns, streaming tokens, multi-modal image inputs, inline tool definitions, and system prompts.

### Endpoint Details
- **URL:** `http://127.0.0.1:11434/api/chat`
- **Method:** `POST`
- **Headers:** `Content-Type: application/json`

---

### Request Payload Specification

```json
{
  "model": "qwen2.5-7b:latest",
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful AI assistant."
    },
    {
      "role": "user",
      "content": "Analyze this chart and summarize the sales trends.",
      "images": [
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
      ]
    }
  ],
  "stream": true,
  "options": {
    "num_predict": 2048,
    "temperature": 0.7,
    "top_p": 0.9
  }
}
```

#### Field Descriptions
| Field | Type | Required | Description |
|---|---|---|---|
| `model` | `string` | Yes | Name of the local Loom model (e.g. `qwen2.5-7b:latest`, `llama3.1:latest`). |
| `messages` | `array` | Yes | Ordered array of message objects (`role`, `content`, optional `images`). |
| `stream` | `boolean` | No | Set `true` to stream responses as Server-Sent Events / Chunked JSON (Default: `true`). |
| `tools` | `array` | No | List of JSON-schema function tool definitions. |
| `options` | `object` | No | Generation hyperparameters (`num_predict`, `temperature`, `top_p`, `num_ctx`). |

---

### Response Specifications

#### A. Streaming Response (`stream: true`)
When streaming, Loom outputs line-delimited JSON objects:

```json
{"model":"qwen2.5-7b:latest","created_at":"2026-08-17T13:19:00Z","message":{"role":"assistant","content":"The"},"done":false}
{"model":"qwen2.5-7b:latest","created_at":"2026-08-17T13:19:01Z","message":{"role":"assistant","content":" chart shows"},"done":false}
{"model":"qwen2.5-7b:latest","created_at":"2026-08-17T13:19:02Z","message":{"role":"assistant","content":" a 15% increase."},"done":true,"total_duration":1245000000,"eval_count":24}
```

#### B. Non-Streaming Response (`stream: false`)
```json
{
  "model": "qwen2.5-7b:latest",
  "created_at": "2026-08-17T13:19:02Z",
  "message": {
    "role": "assistant",
    "content": "The chart shows a steady 15% upward trend in Q3 sales."
  },
  "done": true,
  "total_duration": 1245000000,
  "load_duration": 150000000,
  "prompt_eval_count": 42,
  "eval_count": 24
}
```

---

## 2. OpenAI-Compatible Chat Endpoint (`POST /v1/chat/completions`)

For drop-in compatibility with OpenAI SDKs and third-party tools, Loom provides an OpenAI-compatible REST endpoint.

### Endpoint Details
- **URL:** `http://127.0.0.1:11434/v1/chat/completions`
- **Method:** `POST`
- **Headers:** `Content-Type: application/json`

### Example Request (`cURL`)

```bash
curl http://127.0.0.1:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen2.5-7b:latest",
    "messages": [
      {"role": "system", "content": "You are a financial analyst."},
      {"role": "user", "content": "What is net revenue?"}
    ],
    "temperature": 0.5
  }'
```

### Example Response (`JSON`)

```json
{
  "id": "chatcmpl-987654",
  "object": "chat.completion",
  "created": 1771234567,
  "model": "qwen2.5-7b:latest",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Net revenue is total sales minus returns, allowances, and discounts."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 18,
    "completion_tokens": 14,
    "total_tokens": 32
  }
}
```

---

## 3. Inline Tool Calls via HTTP

When passing custom tools inside the HTTP chat request, the model returns a structured `tool_calls` object if it decides to invoke a function.

### Request Payload with Inline Tool Definition

```json
{
  "model": "qwen2.5-7b:latest",
  "messages": [
    {"role": "user", "content": "What is the weather in London?"}
  ],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Fetch weather conditions for a city",
        "parameters": {
          "type": "object",
          "properties": {
            "city": { "type": "string" }
          },
          "required": ["city"]
        }
      }
    }
  ],
  "stream": false
}
```

### Response Payload with `tool_calls`

```json
{
  "model": "qwen2.5-7b:latest",
  "message": {
    "role": "assistant",
    "content": "",
    "tool_calls": [
      {
        "id": "call_12345",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": {
            "city": "London"
          }
        }
      }
    ]
  },
  "done": true
}
```

---

## 4. Code Examples (Python & JavaScript)

### Python (`requests`)

```python
import requests
import json

url = "http://127.0.0.1:11434/api/chat"
payload = {
    "model": "qwen2.5-7b:latest",
    "messages": [
        {"role": "user", "content": "Explain quantum computing in one sentence."}
    ],
    "stream": False
}

response = requests.post(url, json=payload)
data = response.json()
print("Assistant Response:", data["message"]["content"])
```

### Node.js (`fetch`)

```javascript
async function sendChatRequest() {
  const response = await fetch('http://127.0.0.1:11434/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: 'qwen2.5-7b:latest',
      messages: [{ role: 'user', content: 'Summarize recent market trends.' }],
      stream: false
    })
  });

  const data = await response.json();
  console.log('Response:', data.message.content);
}

sendChatRequest();
```
