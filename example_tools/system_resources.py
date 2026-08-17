import asyncio
import json
import logging
import shutil
import sys
import websockets

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] system_resources: %(message)s")
logger = logging.getLogger(__name__)

LOOM_WS_URL = "ws://localhost:11434/api/tools/interface"
AUTH_TOKEN = "abc"
TOOL_NAME = "system_resources"

tool_schema = {
    "type": "function",
    "function": {
        "name": "system_resources",
        "description": "Get current disk space allocation and basic host platform telemetry.",
        "parameters": {
            "type": "object",
            "properties": {},
            "required": []
        }
    }
}

def get_resources():
    total, used, free = shutil.disk_usage("/")
    return {
        "disk": {
            "total_gb": round(total / (2**30), 2),
            "used_gb": round(used / (2**30), 2),
            "free_gb": round(free / (2**30), 2),
            "percent_used": round((used / total) * 100, 2)
        },
        "platform": sys.platform,
        "python_version": sys.version
    }

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
                        
                        logger.info(f"Executing request {req_id}")
                        try:
                            result = get_resources()
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
