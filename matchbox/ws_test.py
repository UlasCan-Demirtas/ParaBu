import asyncio
import websockets
import json

async def hello():
    async with websockets.connect("ws://localhost:8080/ws") as websocket:
        await websocket.send(json.dumps({"action": "subscribe", "topics": ["depth:BTC/USDT"]}))
        for _ in range(5):
            msg = await websocket.recv()
            print(msg)

asyncio.run(hello())
