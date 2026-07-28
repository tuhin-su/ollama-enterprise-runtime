# Tool Contract

To ensure the Model Server can seamlessly communicate with any arbitrary tool, all tools must adhere to this standard WebSocket contract.

## 1. Connection & Registration (Dynamic Discovery)
When a tool connects to the Model Server's WebSocket, it **must** immediately send a registration message. 

Crucially, the tool **must provide a schema** of its available actions. This ensures the AI model automatically "knows how to use the tool" without hardcoded logic.

**Payload:**
```json
{
  "type": "register",
  "name": "ToolName",
  "description": "A brief description of what this tool does.",
  "actions_schema": {
    "action_name": {
      "description": "What this specific action does.",
      "parameters": {
        "arg1": "type (e.g., string, int, boolean)",
        "arg2": "type"
      }
    }
  }
}
```

## 2. Receiving Commands (Model -> Tool)
The tool must listen for incoming JSON messages where `"type" == "command"`. 

**Payload from Server:**
```json
{
  "type": "command",
  "action": "action_name",
  "payload": {
    "arg1": "value1"
  }
}
```

The tool **must** process the action and return a `command_result` message.

**Response from Tool:**
```json
{
  "type": "command_result",
  "action": "action_name",
  "result": {
    "status": "success",
    "data": "..."
  }
}
```

## 3. Emitting Proactive Events (Tool -> Model)
Tools can take initiative and send alerts, telemetry, or notifications to the model at any time without being prompted.

**Payload from Tool:**
```json
{
  "type": "alert",
  "data": {
    "level": "info|warning|error",
    "message": "Human readable description of the event"
  }
}
```
