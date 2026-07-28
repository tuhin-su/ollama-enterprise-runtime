import asyncio
import json
import websockets

class IntelligentAIModelServer:
    """
    This server represents an LLM that dynamically learns how to use tools
    based on the JSON schema they provide upon connecting.
    """
    def __init__(self):
        self.connected_tools = {}

    async def register(self, websocket):
        try:
            init_message = await websocket.recv()
            data = json.loads(init_message)
            if data.get("type") == "register":
                tool_name = data.get("name")
                schema = data.get("actions_schema", {})
                
                self.connected_tools[tool_name] = {
                    "ws": websocket,
                    "schema": schema,
                    "description": data.get("description", "")
                }
                
                print(f"\n[AI Model] Discovered new tool: {tool_name}")
                print(f"[AI Model] Purpose: {self.connected_tools[tool_name]['description']}")
                print(f"[AI Model] Learned how to use the following actions:")
                for action, details in schema.items():
                    print(f"  - {action}: {details}")
                
                return tool_name
        except Exception as e:
            print(f"[Server] Registration error: {e}")
        return None

    async def simulate_ai_decision(self, tool_name, event):
        """
        Simulates an LLM 'thinking' about an event and looking at the 
        tool's schema to decide what action to take dynamically.
        """
        tool_data = self.connected_tools[tool_name]
        schema = tool_data["schema"]
        
        print(f"\n[AI Model Thinking] I received an event from {tool_name}: {event}")
        
        # Example dynamic decision logic:
        if "weather" in tool_name.lower() and event.get("data", {}).get("level") == "warning":
            # AI looks at the schema to figure out what it can do
            if "get_current_weather" in schema:
                print("[AI Model Thinking] I need to verify this storm. I see 'get_current_weather' in the schema.")
                return {
                    "type": "command",
                    "action": "get_current_weather",
                    "payload": {"location": "Space Station Alpha"}
                }
        
        return None

    async def handle_client(self, websocket):
        tool_name = await self.register(websocket)
        if not tool_name:
            return

        try:
            async for message in websocket:
                event = json.loads(message)
                
                # If the tool sends a proactive alert
                if event.get("type") == "alert":
                    command = await self.simulate_ai_decision(tool_name, event)
                    
                    if command:
                        await websocket.send(json.dumps(command))
                        print(f"[AI Model -> {tool_name}] Dynamically invoked: {command['action']}")
                
                # If the tool responds to our command
                elif event.get("type") == "command_result":
                    print(f"[AI Model] Received result from {tool_name}: {event['result']}")
                    
        except websockets.exceptions.ConnectionClosed:
            print(f"[Server] Tool Disconnected: {tool_name}")
        finally:
            if tool_name in self.connected_tools:
                del self.connected_tools[tool_name]

async def main():
    server_logic = IntelligentAIModelServer()
    server = await websockets.serve(server_logic.handle_client, "localhost", 8765)
    print("Intelligent AI Model Server started on ws://localhost:8765")
    await asyncio.Future()

if __name__ == "__main__":
    asyncio.run(main())
