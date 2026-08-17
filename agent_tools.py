import asyncio
import json
import logging
import sys
import os
import subprocess
import tempfile
import urllib.request
import shutil
import websockets
from typing import Dict, Any

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)

LOOM_WS_URL = "ws://localhost:11434/api/tools/interface"
# Note: globalToolServer.AuthToken is hardcoded to "abc" in server/tool_interface.go
AUTH_TOKEN = "abc"

async def register_and_run_tool(tool_name: str, tool_schema: Dict[str, Any], handler):
    """Connects to the Loom server, registers a tool with its schema, and runs its handler."""
    while True:
        try:
            logger.info(f"Connecting to Loom tool interface at {LOOM_WS_URL} for tool '{tool_name}'...")
            async with websockets.connect(LOOM_WS_URL) as websocket:
                # 1. Authenticate and register the tool schema
                register_msg = {
                    "auth_token": AUTH_TOKEN,
                    "role": "tool",
                    "tool_name": tool_name,
                    "tool_schema": tool_schema
                }
                await websocket.send(json.dumps(register_msg))
                
                # Check response
                auth_resp = await websocket.recv()
                logger.info(f"[{tool_name}] Registration status: {auth_resp}")
                
                # 2. Main loop waiting for execute requests
                async for message in websocket:
                    data = json.loads(message)
                    if data.get("type") == "execute_tool":
                        req_id = data.get("request_id")
                        payload = data.get("payload", {})
                        logger.info(f"[{tool_name}] Received execution request {req_id}: {payload}")
                        
                        try:
                            # Run tool logic (async or sync handler)
                            if asyncio.iscoroutinefunction(handler):
                                result = await handler(payload)
                            else:
                                result = handler(payload)
                            response_payload = {"status": "success", "result": result}
                        except Exception as e:
                            logger.error(f"[{tool_name}] Error executing handler: {e}", exc_info=True)
                            response_payload = {"status": "error", "error": str(e)}
                        
                        # Send execution result back
                        response_msg = {
                            "type": "tool_call_model",
                            "request_id": req_id,
                            "source_tool": tool_name,
                            "payload": response_payload
                        }
                        await websocket.send(json.dumps(response_msg))
                        logger.info(f"[{tool_name}] Returned result for {req_id}")
                        
        except Exception as e:
            logger.error(f"[{tool_name}] Connection lost or error occurred: {e}. Retrying in 5 seconds...")
            await asyncio.sleep(5)

# --- Tool 1: Python Sandbox Executor ---
def handle_python_sandbox(payload: Dict[str, Any]) -> Dict[str, Any]:
    """Executes a block of Python code safely in a separate subprocess."""
    code = payload.get("code")
    if not code:
        raise ValueError("Missing 'code' parameter in payload")
    
    logger.info(f"Running code snippet (len={len(code)}) in sandbox subprocess...")
    
    # Write code to temp file and run it
    with tempfile.NamedTemporaryFile(suffix=".py", mode="w", delete=False) as f:
        f.write(code)
        temp_file_path = f.name
        
    try:
        # Enforce a strict timeout of 10 seconds to prevent infinite loops
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

python_sandbox_schema = {
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

# --- Tool 2: Data Analyzer ---
def handle_data_analyzer(payload: Dict[str, Any]) -> Dict[str, Any]:
    """Performs statistical analysis or aggregation on list data."""
    data_list = payload.get("data")
    operation = payload.get("operation", "summary")
    
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
        mean_val = sum(data_list) / n
        variance = sum((x - mean_val) ** 2 for x in data_list) / max(1, n - 1)
        std_dev = variance ** 0.5
        return {
            "count": n,
            "min": sorted_data[0],
            "max": sorted_data[-1],
            "mean": mean_val,
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

data_analyzer_schema = {
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

# --- Tool 3: Web Retriever ---
def handle_web_retriever(payload: Dict[str, Any]) -> Dict[str, Any]:
    """Retrieves plain text content from a public URL safely without extra dependencies."""
    url = payload.get("url")
    if not url:
        raise ValueError("Missing 'url' parameter in payload")
        
    if not (url.startswith("http://") or url.startswith("https://")):
        raise ValueError("URL must start with http:// or https://")
        
    logger.info(f"Retrieving content from url: {url}")
    
    try:
        req = urllib.request.Request(
            url, 
            headers={'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)'}
        )
        with urllib.request.urlopen(req, timeout=8.0) as response:
            content = response.read().decode('utf-8', errors='ignore')
            # Extract first 5000 chars to avoid prompt context overload
            truncated = content[:5000]
            return {
                "url": url,
                "length": len(content),
                "content": truncated,
                "is_truncated": len(content) > 5000
            }
    except Exception as e:
        return {
            "url": url,
            "error": f"Failed to download web page: {e}"
        }

web_retriever_schema = {
    "type": "function",
    "function": {
        "name": "web_retriever",
        "description": "Download plain text content from a web page (URL). Useful for grabbing news, document articles, or web data.",
        "parameters": {
            "type": "object",
            "properties": {
                "url": {
                    "type": "string",
                    "description": "The HTTP or HTTPS URL to retrieve."
                }
            },
            "required": ["url"]
        }
    }
}

# --- Tool 4: System Resource Monitor ---
def handle_system_resources(payload: Dict[str, Any]) -> Dict[str, Any]:
    """Retrieves basic storage disk usage and host system telemetry."""
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

system_resources_schema = {
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

async def main():
    # Gather execution loops for all four tools concurrently
    await asyncio.gather(
        register_and_run_tool("python_sandbox", python_sandbox_schema, handle_python_sandbox),
        register_and_run_tool("data_analyzer", data_analyzer_schema, handle_data_analyzer),
        register_and_run_tool("web_retriever", web_retriever_schema, handle_web_retriever),
        register_and_run_tool("system_resources", system_resources_schema, handle_system_resources)
    )

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Agent tools terminated by user.")
