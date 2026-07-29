import asyncio
import json
import logging
import websockets

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

OLLAMA_WS_URL = "ws://localhost:11434/api/tools/interface"
AUTH_TOKEN = "abc"

async def tool_runner(tool_name: str, handler, schema: dict):
    """Generic runner for a tool connecting to the Ollama websocket."""
    async with websockets.connect(OLLAMA_WS_URL) as websocket:
        # 1. Authenticate with Priority for Queueing
        auth_msg = {
            "auth_token": AUTH_TOKEN,
            "role": "tool",
            "tool_name": tool_name,
            "tool_schema": schema,
            "priority": 10  # Higher priority for faster queuing
        }
        await websocket.send(json.dumps(auth_msg))
        
        auth_response = await websocket.recv()
        logger.info(f"[{tool_name}] Auth Response: {auth_response}")

        # 2. Listen for requests from the model
        logger.info(f"[{tool_name}] Listening for requests...")
        try:
            async for message in websocket:
                data = json.loads(message)
                
                if data.get("type") == "execute_tool":
                    request_id = data.get("request_id")
                    payload = data.get("payload", {})
                    logger.info(f"[{tool_name}] Received task {request_id}: {payload}")
                    
                    try:
                        result = handler(payload)
                        response_payload = {"status": "success", "result": result}
                    except Exception as e:
                        response_payload = {"status": "error", "error": str(e)}
                    
                    response_msg = {
                        "type": "tool_call_model",
                        "request_id": request_id,
                        "source_tool": tool_name,
                        "payload": response_payload
                    }
                    await websocket.send(json.dumps(response_msg))
                    logger.info(f"[{tool_name}] Sent result {request_id}")
                    
        except websockets.exceptions.ConnectionClosed:
            logger.info(f"[{tool_name}] Connection closed.")


def calculator_handler(payload):
    expression = payload.get("expression")
    if not expression:
        raise ValueError("Missing 'expression' in payload")
    return eval(expression, {"__builtins__": None}, {})

def weather_handler(payload):
    location = payload.get("location")
    if not location:
        raise ValueError("Missing 'location' in payload")
    if "san francisco" in location.lower():
        return {"temperature": 15, "unit": "Celsius", "condition": "Foggy"}
    return {"temperature": 22, "unit": "Celsius", "condition": "Sunny"}

async def main():
    calculator_schema = {
        "type": "function",
        "function": {
            "name": "calculator",
            "description": "Evaluate a mathematical expression",
            "parameters": {
                "type": "object",
                "properties": {
                    "expression": {
                        "type": "string",
                        "description": "The math expression to evaluate, e.g., '2 + 2'"
                    }
                },
                "required": ["expression"]
            }
        }
    }
    
    weather_schema = {
        "type": "function",
        "function": {
            "name": "weather",
            "description": "Get current weather for a location",
            "parameters": {
                "type": "object",
                "properties": {
                    "location": {
                        "type": "string",
                        "description": "The city name, e.g., 'San Francisco'"
                    }
                },
                "required": ["location"]
            }
        }
    }

    await asyncio.gather(
        tool_runner("calculator", calculator_handler, calculator_schema),
        tool_runner("weather", weather_handler, weather_schema)
    )

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Tools stopped by user.")
