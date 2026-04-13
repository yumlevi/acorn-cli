"""Local HTTP server commands."""

from acorn.commands.registry import command
from acorn.tools.serve import start_server, stop_server, list_servers
from rich.text import Text
from rich.panel import Panel
from rich.table import Table
import os


@command('/serve')
async def cmd_serve(args, **ctx):
    app = ctx.get('app')
    if not app:
        return

    t = app.theme_data
    parts = args.strip().split(None, 1)
    subcmd = parts[0] if parts else 'start'
    subargs = parts[1] if len(parts) > 1 else ''

    if subcmd == 'start':
        directory = subargs or app.cwd
        if not os.path.isabs(directory):
            directory = os.path.join(app.cwd, directory)
        result = start_server(directory)
        if result.get('ok'):
            app._log(Text(f'  ✓ Serving {directory} at {result["url"]}', style=t['success']))
        else:
            app._log(Text(f'  ✗ {result.get("error")}', style=t['error']))

    elif subcmd == 'stop':
        port = int(subargs) if subargs.isdigit() else 0
        if not port:
            app._log(Text('  Usage: /serve stop <port>', style=t['muted']))
        else:
            result = stop_server(port)
            if result.get('ok'):
                app._log(Text(f'  ✓ Stopped server on port {port}', style=t['success']))
            else:
                app._log(Text(f'  ✗ {result.get("error")}', style=t['error']))

    elif subcmd == 'list':
        servers = list_servers()
        if not servers:
            app._log(Text('  No local servers running', style=t['muted']))
        else:
            for s in servers:
                app._log(Text(f'  http://localhost:{s["port"]}', style=t['accent']))

    else:
        app._log(Panel(
            '/serve [dir]       Start serving current or specified directory\n'
            '/serve stop <port> Stop a server\n'
            '/serve list        List running servers',
            title='Local Server', border_style=t['accent'], style=f'on {t["bg_panel"]}',
        ))

    app._scroll_bottom()
