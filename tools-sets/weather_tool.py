import asyncio
import json
import websockets
import random

class WeatherTool:
    def __init__(self, websocket):
        self.ws = websocket
        self.current_temp = 72

    async def execute_command(self, action, payload):
        print(f"[WeatherTool] Received command: {action} with {payload}")
        if action == "get_current_weather":
            location = payload.get("location", "Unknown")
            return {"status": "success", "temp": self.current_temp, "location": location}
        return {"status": "error", "message": "Unknown action"}

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
            print("[WeatherTool] Connection to model lost.")

    async def weather_monitoring_loop(self):
        """Proactively monitors weather and alerts the model if a storm approaches."""
        while True:
            await asyncio.sleep(8)
            # Simulate a random storm event
            if random.random() > 0.7:
                self.current_temp -= 10
                alert = {
                    "type": "alert",
                    "data": {
                        "level": "warning", 
                        "message": "Sudden temperature drop detected! Possible storm approaching."
                    }
                }
                print("[WeatherTool] Emitting storm alert to model!")
                await self.ws.send(json.dumps(alert))

async def main():
    uri = "ws://localhost:8765"
    async with websockets.connect(uri) as websocket:
        print("[WeatherTool] Connected to Model Server.")
        
        # Register per the tool_contract.md
        register_msg = {
            "type": "register", 
            "name": "WeatherMonitor",
            "description": "Provides weather data and proactive storm alerts.",
            "actions_schema": {
                "get_current_weather": {
                    "description": "Returns the current temperature for a given location.",
                    "parameters": {
                        "location": "string (e.g., 'San Francisco', 'Space Station')"
                    }
                }
            }
        }
        await websocket.send(json.dumps(register_msg))
        
        tool = WeatherTool(websocket)
        await asyncio.gather(
            tool.listen_for_commands(),
            tool.weather_monitoring_loop()
        )

if __name__ == "__main__":
    asyncio.run(main())
