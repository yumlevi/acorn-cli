"""WebSocket client — auth, connection, message routing."""

import asyncio
import json
import urllib.request
import urllib.error
import websockets


class AuthError(Exception):
    pass


class Connection:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port
        self.ws = None
        self.token = None
        self.tool_executor = None
        self._handlers = {}
        self._receive_task = None

    async def authenticate(self, username: str, key: str) -> str:
        url = f'http://{self.host}:{self.port}/api/acorn/auth'
        payload = json.dumps({'username': username, 'key': key}).encode()
        req = urllib.request.Request(url, data=payload, headers={'Content-Type': 'application/json'}, method='POST')
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                data = json.loads(resp.read())
                self.token = data['token']
                return self.token
        except urllib.error.HTTPError as e:
            body = e.read().decode()
            try:
                data = json.loads(body)
                raise AuthError(data.get('error', f'HTTP {e.code}'))
            except (json.JSONDecodeError, AuthError):
                raise
            raise AuthError(f'HTTP {e.code}: {body[:200]}')

    async def connect(self, token: str):
        url = f'ws://{self.host}:{self.port}/ws?token={token}'
        self.ws = await websockets.connect(url, ping_interval=20, ping_timeout=10, max_size=10 * 1024 * 1024)
        self._receive_task = asyncio.create_task(self._receive_loop())

    async def close(self):
        if self._receive_task:
            self._receive_task.cancel()
        if self.ws:
            await self.ws.close()

    def on(self, msg_type: str, handler):
        self._handlers[msg_type] = handler

    async def send(self, data: str):
        if self.ws:
            await self.ws.send(data)

    async def _receive_loop(self):
        try:
            async for raw in self.ws:
                try:
                    msg = json.loads(raw)
                except json.JSONDecodeError:
                    continue
                msg_type = msg.get('type', '')

                # Handle tool requests from server
                if msg_type == 'tool:request' and self.tool_executor:
                    asyncio.create_task(self._handle_tool_request(msg))
                    continue

                # Route to registered handlers
                handler = self._handlers.get(msg_type)
                if handler:
                    try:
                        await handler(msg)
                    except Exception:
                        pass
        except websockets.ConnectionClosed:
            pass
        except asyncio.CancelledError:
            pass

    async def _handle_tool_request(self, msg: dict):
        tool_id = msg.get('id', '')
        tool_name = msg.get('name', '')
        tool_input = msg.get('input', {})
        try:
            result = await self.tool_executor.execute(tool_name, tool_input)
            await self.send(json.dumps({'type': 'tool:result', 'id': tool_id, 'result': result}))
        except Exception as e:
            await self.send(json.dumps({'type': 'tool:result', 'id': tool_id, 'result': {'error': str(e)}}))
