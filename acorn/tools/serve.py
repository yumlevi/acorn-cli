"""Local HTTP server for serving static files from the project."""

import asyncio
import os
import http.server
import threading
import socket


_servers = {}  # port → (server, thread)


def _find_free_port(start=8080):
    for port in range(start, start + 100):
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            s.bind(('', port))
            s.close()
            return port
        except OSError:
            continue
    return None


def start_server(directory: str, port: int = 0) -> dict:
    """Start a local HTTP server serving files from directory."""
    if not os.path.isdir(directory):
        return {'error': f'Directory not found: {directory}'}

    if port == 0:
        port = _find_free_port()
        if not port:
            return {'error': 'No free port found (tried 8080-8179)'}

    # Check if already serving on this port
    if port in _servers:
        return {'error': f'Port {port} already in use by a local server', 'port': port}

    class Handler(http.server.SimpleHTTPRequestHandler):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=directory, **kwargs)

        def log_message(self, format, *args):
            pass  # Suppress logs

    try:
        server = http.server.HTTPServer(('0.0.0.0', port), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        _servers[port] = (server, thread)
        return {
            'ok': True,
            'port': port,
            'directory': directory,
            'url': f'http://localhost:{port}',
            'note': f'Serving {directory} at http://localhost:{port} — use /serve stop {port} to stop',
        }
    except Exception as e:
        return {'error': str(e)}


def stop_server(port: int) -> dict:
    if port not in _servers:
        return {'error': f'No server running on port {port}'}
    server, thread = _servers.pop(port)
    server.shutdown()
    return {'ok': True, 'note': f'Server on port {port} stopped'}


def list_servers() -> list:
    return [{'port': p, 'directory': s[0].server_address} for p, s in _servers.items()]
