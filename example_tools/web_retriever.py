import asyncio
import json
import logging
import urllib.request
import websockets

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] web_retriever: %(message)s")
logger = logging.getLogger(__name__)

LOOM_WS_URL = "ws://localhost:11434/api/tools/interface"
AUTH_TOKEN = "abc"
TOOL_NAME = "web_retriever"

tool_schema = {
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

def retrieve_url(url: str) -> dict:
    if not (url.startswith("http://") or url.startswith("https://")):
        raise ValueError("URL must start with http:// or https://")
        
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
                        url = payload.get("url", "")
                        
                        logger.info(f"Executing request {req_id}")
                        try:
                            result = retrieve_url(url)
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
