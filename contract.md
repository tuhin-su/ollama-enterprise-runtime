# Ollama Server API Communication Contract

This document defines the interface and protocol contract for any external tool, script, application, or agent (such as [chat.py](file:///home/master/Desktop/ollama-master/chat.py)) communicating with the Ollama server.

---

## 1. Connection Details

### A. Host & Port
By default, the Ollama server binds to the localhost interface on port `11434`.
- **Default Endpoint URL:** `http://127.0.0.1:11434` or `http://localhost:11434`
- **Environment Override:** The environment variable `OLLAMA_HOST` can be set to specify a custom host address and port (e.g. `OLLAMA_HOST=0.0.0.0:11434` or a remote host ip/port).

### B. Health Verification
Before initiating main chat/generation requests, clients should check server availability:
- **Endpoint:** `GET /`
- **Response:** `200 OK` (returns text `"Ollama is running"`)

---

## 2. Authentication & Headers

If Token Authentication is configured on the Ollama server (enabled by setting `api_token` in `~/.ollama/server.json`), the following rules apply:

- **Required Header:** Every API request must carry a Bearer token.
  ```http
  Authorization: Bearer <your_configured_secret_token>
  ```
- **Standard Headers:**
  ```http
  Content-Type: application/json
  Accept: application/json
  ```
- **Unauthorized Status:** If the token is missing or invalid, the server responds with:
  ```http
  HTTP/1.1 401 Unauthorized
  Content-Type: application/json

  {"error": "Unauthorized"}
  ```

---

## 3. Core API Endpoints

### A. List Models (`GET /api/tags`)
Retrieves all locally pulled models.
- **Request:**
  ```http
  GET /api/tags
  ```
- **Response Example (200 OK):**
  ```json
  {
    "models": [
      {
        "name": "deepseek-r1-llama-8b-uncensored:latest",
        "model": "deepseek-r1-llama-8b-uncensored:latest",
        "modified_at": "2026-07-27T22:15:00Z",
        "size": 4700000000,
        "digest": "sha256:..."
      }
    ]
  }
  ```

### B. Chat Completion (`POST /api/chat`)
Initiates a conversational turn with model capability support.

- **Request Fields:**
  - `model` (string, required): Model identifier.
  - `messages` (array of objects, required): Conversation history.
  - `stream` (boolean, optional, default: true): Stream tokens chunk by chunk.
  - `options` (object, optional): Model parameters like `num_ctx`, `temperature`.
  - `keep_alive` (string, optional): Duration to keep model loaded (e.g. `5m`).

- **Non-Streaming Payload Example:**
  ```json
  {
    "model": "deepseek-r1-llama-8b-uncensored",
    "messages": [
      {
        "role": "user",
        "content": "Hello, who are you?"
      }
    ],
    "stream": false
  }
  ```

- **Streaming Response Format (chunks):**
  The server streams JSON objects separated by newlines (`\n`):
  ```json
  {"model":"deepseek-r1-llama-8b-uncensored","created_at":"2026-07-27T23:00:00Z","message":{"role":"assistant","content":"Hi"},"done":false}
  {"model":"deepseek-r1-llama-8b-uncensored","created_at":"2026-07-27T23:00:01Z","message":{"role":"assistant","content":"!"},"done":true}
  ```

---

## 4. Interactive Tool / Function Calling

For models supporting function calling, clients can submit an optional `tools` array:

- **Request Tool Array structure:**
  ```json
  {
    "model": "deepseek-r1-llama-8b-uncensored",
    "messages": [...],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_current_weather",
          "description": "Get the current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "City name, e.g. London"
              }
            },
            "required": ["location"]
          }
        }
      }
    ]
  }
  ```

If the model decides to invoke a tool, the response payload returns a list of tool calls:
- **Response format containing tool call:**
  ```json
  {
    "message": {
      "role": "assistant",
      "tool_calls": [
        {
          "id": "call_12345",
          "type": "function",
          "function": {
            "name": "get_current_weather",
            "arguments": {"location": "London"}
          }
        }
      ]
    },
    "done": true
  }
  ```

Clients execute the tool locally and post back the result in the subsequent chat turn:
- **Tool Result payload:**
  ```json
  {
    "role": "tool",
    "content": "{\"temp\": 15, \"condition\": \"Rainy\"}",
    "tool_call_id": "call_12345"
  }
  ```
