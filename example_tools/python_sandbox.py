import asyncio
import json
import logging
import sys
import os
import subprocess
import tempfile
import websockets

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] python_sandbox: %(message)s")
logger = logging.getLogger(__name__)

OLLAMA_WS_URL = "ws://localhost:11434/api/tools/interface"
AUTH_TOKEN = "abc"
TOOL_NAME = "python_sandbox"

tool_schema = {
    "type": "function",
    "function": {
        "name": "python_sandbox",
        "description": "Execute a block of Python code in a local subprocess and return stdout, stderr, and exit code. Best for running tests, calculations, and data processing.",
        "parameters": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string",
                    "description": "The exact Python code to run. Output should be printed to stdout."
                }
            },
            "required": ["code"]
        }
    }
}

def execute_code(code: str) -> dict:
    with tempfile.NamedTemporaryFile(suffix=".py", mode="w", delete=False) as f:
        f.write(code)
        temp_file_path = f.name
        
    try:
        proc = subprocess.run(
            [sys.executable, temp_file_path],
            capture_output=True,
            text=True,
            timeout=10.0
        )
        return {
            "stdout": proc.stdout,
            "stderr": proc.stderr,
            "exit_code": proc.returncode
        }
    except subprocess.TimeoutExpired:
        return {
            "stdout": "",
            "stderr": "Execution timed out (maximum limit is 10 seconds).",
            "exit_code": -9
        }
    finally:
        if os.path.exists(temp_file_path):
            os.remove(temp_file_path)

async def main():
    while True:
        try:
            logger.info(f"Connecting to Ollama tool interface at {OLLAMA_WS_URL}...")
            async with websockets.connect(OLLAMA_WS_URL) as websocket:
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
                        code = payload.get("code", "")
                        
                        logger.info(f"Executing request {req_id}")
                        try:
                            result = execute_code(code)
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
