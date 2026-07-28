import asyncio
import json
import websockets
import math

class CalculatorTool:
    def __init__(self, websocket):
        self.ws = websocket

    async def execute_command(self, action, payload):
        print(f"[Calculator] Received command: {action} with {payload}")
        try:
            if action == "add":
                return {"status": "success", "result": payload["a"] + payload["b"]}
            elif action == "multiply":
                return {"status": "success", "result": payload["a"] * payload["b"]}
            elif action == "sqrt":
                return {"status": "success", "result": math.sqrt(payload["a"])}
            else:
                return {"status": "error", "message": "Unknown action"}
        except KeyError as e:
            return {"status": "error", "message": f"Missing argument: {e}"}

    async def listen_for_commands(self):
        try:
            async for message in self.ws:
                data = json.loads(message)
                if data.get("type") == "command":
                    result = await self.execute_command(data["action"], data.get("payload", {}))
                    response = {
                        "type": "command_result",
                        "action": data["action"],
                        "result": result
                    }
                    await self.ws.send(json.dumps(response))
        except websockets.exceptions.ConnectionClosed:
            print("[Calculator] Connection to model lost.")

async def main():
    uri = "ws://localhost:8765"
    async with websockets.connect(uri) as websocket:
        print("[Calculator] Connected to Model Server.")
        
        # Register per the tool_contract.md
        register_msg = {
            "type": "register", 
            "name": "Calculator",
            "description": "Performs basic mathematical operations.",
            "actions_schema": {
                "add": {
                    "description": "Adds two numbers together.",
                    "parameters": {"a": "float", "b": "float"}
                },
                "multiply": {
                    "description": "Multiplies two numbers.",
                    "parameters": {"a": "float", "b": "float"}
                },
                "sqrt": {
                    "description": "Calculates the square root of a number.",
                    "parameters": {"a": "float"}
                }
            }
        }
        await websocket.send(json.dumps(register_msg))
        
        tool = CalculatorTool(websocket)
        await tool.listen_for_commands()

if __name__ == "__main__":
    asyncio.run(main())
