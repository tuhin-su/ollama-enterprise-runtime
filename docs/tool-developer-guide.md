# Ollama Developer Guide: Tools and Client Applications

Welcome to the Ollama developer guide. This document explains how you can develop rich client applications and integrate tools with Ollama's agentic backend.

## 1. Developing Tools

Ollama supports a native tool architecture. This means models can invoke specific functions that you define, which is incredibly useful for connecting models to APIs, internal databases, or local filesystems.

### Defining a Tool

When communicating with the `/api/chat` or `/v1/chat/completions` endpoint, you can pass a list of `tools`. A tool definition strictly follows the JSON Schema format.

**Example Tool Definition (JSON):**
```json
{
  "type": "function",
  "function": {
    "name": "get_weather",
    "description": "Get the current weather for a specific location.",
    "parameters": {
      "type": "object",
      "properties": {
        "location": {
          "type": "string",
          "description": "The city and state, e.g. San Francisco, CA"
        },
        "unit": {
          "type": "string",
          "enum": ["celsius", "fahrenheit"]
        }
      },
      "required": ["location"]
    }
  }
}
```

### Handling Tool Calls

When a model decides to invoke a tool, its response will include a `tool_calls` block. Your client application must:
1. Parse the tool call.
2. Execute the underlying logic on the client side (e.g., call a weather API).
3. Append a new message to the conversation history with `role: "tool"` and the output of the function.
4. Send the updated conversation back to Ollama to synthesize the final response.

## 2. Developing Client Applications

Client applications can interact with Ollama via its robust REST API or official SDKs (`ollama-python` and `ollama-js`).

### Leveraging Agentic Routing

Ollama's backend automatically parses the `DESCRIPTION` of your local models. If you are building an application that needs specialized capabilities (like vision, math, or coding), you do not need to hardcode specific model names (like `qwen2.5-vl-7b:latest`). 

Instead, your client can rely on Ollama's native model chaining:
- Ensure your specialized models are loaded locally and have descriptive `DESCRIPTION` fields in their `Modelfiles` (e.g., `DESCRIPTION A vision processing model`).
- Your client application can simply invoke the default model, and the backend orchestrator will dynamically route tasks to the specialist models as needed via the `chain_request` mechanism.

### Example: Python Client using Tools

Here is a simple python snippet demonstrating how a client application can pass a tool to Ollama:

```python
from ollama import chat

# 1. Define the tool
weather_tool = {
    'type': 'function',
    'function': {
        'name': 'get_current_weather',
        'description': 'Get the current weather',
        'parameters': {
            'type': 'object',
            'properties': {
                'location': {
                    'type': 'string',
                    'description': 'The city and state, e.g. San Francisco, CA',
                },
            },
            'required': ['location'],
        },
    },
}

# 2. Invoke the model
response = chat(
    model='gemma4',
    messages=[{'role': 'user', 'content': 'What is the weather in Paris?'}],
    tools=[weather_tool]
)

# 3. Process the tool call
if response.message.tool_calls:
    for tool_call in response.message.tool_calls:
        if tool_call.function.name == 'get_current_weather':
            location = tool_call.function.arguments.get('location')
            print(f"Model wants to check weather for: {location}")
            # ... perform API request ...
```
