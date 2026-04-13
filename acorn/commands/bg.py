"""Background process commands."""

from acorn.commands.registry import command
from rich.text import Text
from rich.panel import Panel
from rich.table import Table


@command('/bg')
async def cmd_bg(args, **ctx):
    app = ctx.get('app')
    if not app:
        return

    t = app.theme_data
    parts = args.strip().split(None, 1)
    subcmd = parts[0] if parts else ''
    subargs = parts[1] if len(parts) > 1 else ''

    if not subcmd or subcmd == 'list':
        # List all background processes
        procs = app.process_manager.list_all()
        if not procs:
            app._log(Text('  No background processes', style=t['muted']))
            app._scroll_bottom()
            return

        table = Table.grid(padding=(0, 2))
        table.add_column(style=f'bold {t["accent"]}', min_width=4)   # ID
        table.add_column(min_width=8)   # status
        table.add_column(min_width=8)   # elapsed
        table.add_column()              # command

        for bp in procs:
            status = Text('● running', style=t['success']) if bp.running else Text('✓ done', style=t['muted'])
            if bp.exit_code and bp.exit_code != 0:
                status = Text(f'✗ exit {bp.exit_code}', style=t['error'])
            table.add_row(
                f'#{bp.id}',
                status,
                bp.elapsed,
                Text(bp.command[:60], style=t['fg']),
            )

        app._log(Panel(table, title='Background Processes', border_style=t['accent'],
                        style=f'on {t["bg_panel"]}'))
        app._scroll_bottom()

    elif subcmd == 'run' and subargs:
        # Launch a command in background
        bp = await app.process_manager.launch(subargs, app.cwd)
        app._log(Text(f'  ⚡ Background #{bp.id}: {subargs}', style=t['success']))
        app._update_footer()
        app._scroll_bottom()

    elif subcmd == 'view' or subcmd.isdigit():
        # View output of a process
        pid = int(subcmd) if subcmd.isdigit() else int(subargs) if subargs.isdigit() else 0
        bp = app.process_manager.get(pid)
        if not bp:
            app._log(Text(f'  No process #{pid}', style='red'))
            app._scroll_bottom()
            return

        status = '● running' if bp.running else f'✓ done (exit {bp.exit_code})'
        output = '\n'.join(bp.output) if bp.output else '(no output yet)'
        # Truncate for display
        if len(output) > 5000:
            output = output[-5000:]

        app._log(Panel(
            Text(output, style=t['fg']),
            title=f'#{bp.id} {bp.command[:40]} — {status} ({bp.elapsed})',
            border_style=t['accent'] if bp.running else t['muted'],
            style=f'on {t["bg_panel"]}',
            padding=(0, 1),
        ))
        app._scroll_bottom()

    elif subcmd == 'kill' and subargs:
        # Kill a process
        pid = int(subargs) if subargs.isdigit() else 0
        if app.process_manager.kill(pid):
            app._log(Text(f'  Killed #{pid}', style=t['warning']))
        else:
            app._log(Text(f'  Cannot kill #{pid} (not running or not found)', style='red'))
        app._scroll_bottom()

    elif subcmd == 'rm' and subargs:
        # Remove a finished process
        pid = int(subargs) if subargs.isdigit() else 0
        if app.process_manager.remove(pid):
            app._log(Text(f'  Removed #{pid}', style=t['muted']))
        else:
            app._log(Text(f'  Cannot remove #{pid} (still running or not found)', style='red'))
        app._scroll_bottom()

    else:
        app._log(Panel(
            '/bg                  List all processes\n'
            '/bg run <cmd>        Run command in background\n'
            '/bg <id>             View process output\n'
            '/bg kill <id>        Kill a running process\n'
            '/bg rm <id>          Remove a finished process',
            title='Background Processes',
            border_style=t['accent'],
            style=f'on {t["bg_panel"]}',
        ))
        app._scroll_bottom()
