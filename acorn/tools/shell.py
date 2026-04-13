"""Local shell command execution with background support."""

import asyncio


DANGEROUS_PATTERNS = [
    'rm -rf /', 'mkfs', '> /dev/sd', ':(){:|:&};:', 'chmod -R 777 /',
]

# Commands that are likely long-running (servers, watchers, etc.)
BACKGROUND_HINTS = [
    'npm start', 'npm run dev', 'npm run serve', 'yarn start', 'yarn dev',
    'python -m http.server', 'python manage.py runserver',
    'node server', 'nodemon', 'next dev', 'vite',
    'flask run', 'uvicorn', 'gunicorn', 'cargo run',
    'docker compose up', 'docker-compose up',
    'tail -f', 'watch ',
]


async def execute(input: dict, cwd: str, process_manager=None) -> dict:
    command = input.get('command', '')
    timeout_ms = min(input.get('timeout', 120000), 600000)
    timeout = timeout_ms / 1000
    background = input.get('background', False)

    for pattern in DANGEROUS_PATTERNS:
        if pattern in command:
            return {'error': f'Blocked dangerous command pattern: {pattern}'}

    # Auto-detect likely long-running commands → suggest background
    is_server_like = any(hint in command for hint in BACKGROUND_HINTS)

    # If explicitly background or detected as server-like with a process manager
    if (background or is_server_like) and process_manager:
        bp = await process_manager.launch(command, cwd)
        # Wait briefly for early output or crash
        await asyncio.sleep(1.0)
        early_output = '\n'.join(bp.output) if bp.output else '(started, no output yet)'
        if not bp.running:
            return {
                'output': early_output,
                'exitCode': bp.exit_code,
                'note': f'Process exited immediately (exit {bp.exit_code})',
            }
        return {
            'output': early_output[:2000],
            'backgrounded': True,
            'processId': bp.id,
            'note': f'Running in background as #{bp.id}. Use /bg {bp.id} to view output, /bg kill {bp.id} to stop.',
        }

    try:
        proc = await asyncio.create_subprocess_shell(
            command, cwd=cwd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=timeout)
        output = stdout.decode('utf-8', errors='replace')
        if len(output) > 8000:
            mid = len(output) - 8000
            output = output[:4000] + f'\n\n[... {mid} chars truncated ...]\n\n' + output[-4000:]
        return {'output': output, 'exitCode': proc.returncode}
    except asyncio.TimeoutError:
        # On timeout, move to background instead of killing if we have a process manager
        if process_manager:
            bp = await process_manager.launch(command, cwd)
            # Kill the timed-out process (it's a different one)
            try:
                proc.kill()
            except Exception:
                pass
            return {
                'output': f'Command timed out after {timeout}s — moved to background as #{bp.id}',
                'backgrounded': True,
                'processId': bp.id,
                'note': f'Use /bg {bp.id} to view output, /bg kill {bp.id} to stop.',
            }
        try:
            proc.kill()
        except Exception:
            pass
        return {'error': f'Command timed out after {timeout}s', 'exitCode': -1}
    except Exception as e:
        return {'error': str(e)}
