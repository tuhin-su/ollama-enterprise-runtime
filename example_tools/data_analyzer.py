import asyncio
import json
import logging
import websockets

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] data_analyzer: %(message)s")
logger = logging.getLogger(__name__)

LOOM_WS_URL = "ws://localhost:11434/api/tools/interface"
AUTH_TOKEN = "abc"
TOOL_NAME = "data_analyzer"

tool_schema = {
    "type": "function",
    "function": {
        "name": "data_analyzer",
        "description": "Perform basic statistical operations (summary, mean, median, min_max) on a flat list of numeric data.",
        "parameters": {
            "type": "object",
            "properties": {
                "data": {
                    "type": "array",
                    "items": {
                        "type": "number"
                    },
                    "description": "List of integers or floats to analyze."
                },
                "operation": {
                    "type": "string",
                    "enum": ["summary", "mean", "median", "min_max"],
                    "description": "The target statistical operation (default is summary)."
                }
            },
            "required": ["data"]
        }
    }
}

def analyze_data(data_list, operation="summary"):
    if not isinstance(data_list, list):
        raise ValueError("'data' parameter must be a list of numbers")
    if not all(isinstance(x, (int, float)) for x in data_list):
        raise ValueError("All items in 'data' must be integers or floats")
    if not data_list:
        return {"count": 0, "operation": operation, "result": None}

    sorted_data = sorted(data_list)
    n = len(sorted_data)
    
    if operation == "mean" or operation == "summary":
        mean = sum(data_list) / n
    if operation == "median" or operation == "summary":
        if n % 2 == 1:
            median = sorted_data[n // 2]
        else:
            median = (sorted_data[(n // 2) - 1] + sorted_data[n // 2]) / 2.0
            
    if operation == "summary":
        variance = sum((x - mean) ** 2 for x in data_list) / max(1, n - 1)
        std_dev = variance ** 0.5
        return {
            "count": n,
            "min": sorted_data[0],
            "max": sorted_data[-1],
            "mean": mean,
            "median": median,
            "std_dev": std_dev,
            "sum": sum(data_list)
        }
    elif operation == "mean":
        return {"mean": mean}
    elif operation == "median":
        return {"median": median}
    elif operation == "min_max":
        return {"min": sorted_data[0], "max": sorted_data[-1]}
    else:
        raise ValueError(f"Unknown operation: {operation}")

async def main():
    while True:
        try:
            logger.info(f"Connecting to Loom tool interface at {LOOM_WS_URL}...")
            async with websockets.connect(LOOM_WS_URL) as websocket:
                register_msg = {
                    "auth_token": AUTH_TOKEN,
                    "role": "tool",
                    "tool_name": TOOL_NAME,
                    "tool_schema": tool_schema
                }
                await websocket.send(json.dumps(register_msg))
                auth_resp = await websocket.recv()
                logger.info(f"Registration response: {auth_resp}")
                
                async for message in websocket:
                    data = json.loads(message)
                    if data.get("type") == "execute_tool":
                        req_id = data.get("request_id")
                        payload = data.get("payload", {})
                        data_list = payload.get("data", [])
                        op = payload.get("operation", "summary")
                        
                        logger.info(f"Executing request {req_id}")
                        try:
                            result = analyze_data(data_list, op)
                            response_payload = {"status": "success", "result": result}
                        except Exception as e:
                            response_payload = {"status": "error", "error": str(e)}
                            
                        response_msg = {
                            "type": "tool_call_model",
                            "request_id": req_id,
                            "source_tool": TOOL_NAME,
                            "payload": response_payload
                        }
                        await websocket.send(json.dumps(response_msg))
                        logger.info(f"Sent result for {req_id}")
        except Exception as e:
            logger.error(f"Connection error: {e}. Retrying in 5 seconds...")
            await asyncio.sleep(5)

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Stopped.")
