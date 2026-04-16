"""Local shell command execution with background support + sandbox."""

import asyncio
import os
import re


DANGEROUS_PATTERNS = [
    'rm -rf /', 'mkfs', '> /dev/sd', ':(){:|:&};:', 'chmod -R 777 /',
]

# Known-safe commands that don't need extra approval in auto mode
SAFE_COMMANDS = {
    'ls', 'cat', 'head', 'tail', 'wc', 'sort', 'uniq', 'grep', 'find', 'which',
    'echo', 'pwd', 'whoami', 'date', 'uname', 'env', 'printenv', 'id',
    'git', 'node', 'npm', 'npx', 'yarn', 'pnpm', 'bun', 'deno',
    'python', 'python3', 'pip', 'pip3', 'uv',
    'go', 'cargo', 'rustc', 'java', 'javac', 'mvn', 'gradle',
    'ruby', 'gem', 'php', 'composer',
    'docker', 'kubectl', 'terraform',
    'make', 'cmake', 'gcc', 'g++', 'clang',
    'curl', 'wget', 'ssh', 'scp', 'rsync',
    'tar', 'zip', 'unzip', 'gzip', 'gunzip',
    'sed', 'awk', 'cut', 'tr', 'diff', 'patch',
    'mkdir', 'touch', 'cp', 'mv', 'ln',
    'chmod', 'chown',
    'sqlite3', 'psql', 'mysql', 'redis-cli', 'mongosh',
    'ffmpeg', 'convert', 'jq', 'yq',
    'nginx', 'systemctl', 'journalctl',
    'tree', 'file', 'stat', 'du', 'df', 'free', 'top', 'htop', 'ps',
    'nvidia-smi', 'rocm-smi',
    'tsc', 'eslint', 'prettier', 'jest', 'vitest', 'pytest', 'mocha',
    'kill',  # SIGTERM is fine, kill -9 caught by permissions
    'seq', 'sleep', 'true', 'false', 'test',
    'sh', 'bash', 'zsh',
}

# Paths that commands should not reference
BLOCKED_PATHS = [
    '/etc/shadow', '/etc/passwd-', '/etc/sudoers',
    '~/.ssh/id_', '~/.ssh/authorized_keys',
    '~/.gnupg', '~/.aws/credentials', '~/.kube/config',
]

def _build_blocked_patterns():
    patterns = []
    for p in BLOCKED_PATHS:
        # Match both ~ and expanded home
        patterns.append(re.compile(re.escape(p)))
        expanded = p.replace('~', os.path.expanduser('~'))
        if expanded != p:
            patterns.append(re.compile(re.escape(expanded)))
    return patterns

BLOCKED_PATH_RE = _build_blocked_patterns()

# Commands that are likely long-running (servers, watchers, etc.)
BACKGROUND_HINTS = [
    'npm start', 'npm run dev', 'npm run serve', 'yarn start', 'yarn dev',
    'python -m http.server', 'python manage.py runserver',
    'node server', 'nodemon', 'next dev', 'vite',
    'flask run', 'uvicorn', 'gunicorn', 'cargo run',
    'docker compose up', 'docker-compose up',
    'tail -f', 'watch ',
]


def get_command_binary(command: str) -> str:
    """Extract the first binary/command from a shell command string."""
    # Handle cd, env prefix, sudo, etc.
    cmd = command.strip()
    for prefix in ('sudo ', 'env ', 'nice ', 'nohup ', 'time '):
        if cmd.startswith(prefix):
            cmd = cmd[len(prefix):]
    # Handle VAR=val prefix
    while '=' in cmd.split()[0] if cmd.split() else False:
        cmd = cmd.split(None, 1)[1] if ' ' in cmd else cmd
    binary = cmd.split()[0] if cmd.split() else ''
    # Strip path
    if '/' in binary:
        binary = binary.rsplit('/', 1)[-1]
    return binary


def check_path_safety(command: str) -> str:
    """Check if command references sensitive paths. Returns error string or empty."""
    for r in BLOCKED_PATH_RE:
        if r.search(command):
            return f'Command references sensitive path: {r.pattern}'
    return ''


def _handle_bg_command(args: str, pm) -> dict:
    """Handle /bg subcommands: list, read <id>, kill <id>."""
    if not args or args == 'list':
        procs = pm.list_all()
        if not procs:
            return {'output': 'No background processes'}
        lines = []
        for bp in procs:
            status = 'running' if bp.running else f'exited ({bp.exit_code})'
            lines.append(f'#{bp.id}  {status}  {bp.elapsed}  {bp.command[:80]}')
        return {'output': '\n'.join(lines)}

    parts = args.split(None, 1)
    subcmd = parts[0]

    if subcmd == 'kill' and len(parts) > 1:
        try:
            pid = int(parts[1])
        except ValueError:
            return {'error': f'Invalid process ID: {parts[1]}'}
        if pm.kill(pid):
            return {'output': f'Killed #{pid}'}
        return {'error': f'Process #{pid} not found or already stopped'}

    # Default: treat as process ID to read output
    try:
        pid = int(subcmd)
    except ValueError:
        return {'error': f'Usage: /bg [list | <id> | kill <id>]'}

    bp = pm.get(pid)
    if not bp:
        return {'error': f'Process #{pid} not found'}

    output_lines = list(bp.output)
    status = 'running' if bp.running else f'exited ({bp.exit_code})'
    header = f'#{bp.id} [{status}] {bp.elapsed} — {bp.command[:80]}'
    if not output_lines:
        return {'output': f'{header}\n(no output captured)'}
    body = '\n'.join(output_lines[-100:])
    if len(output_lines) > 100:
        body = f'... ({len(output_lines) - 100} earlier lines)\n{body}'
    return {'output': f'{header}\n{body}'}


def _open_exec_log(log_dir, command):
    """Open a live log file for exec output. Returns (file_handle, log_path) or (None, None)."""
    if not log_dir:
        return None, None
    try:
        import time
        from pathlib import Path
        Path(log_dir).mkdir(parents=True, exist_ok=True)
        ts = time.strftime('%H%M%S')
        slug = re.sub(r'[^a-zA-Z0-9]', '-', command.split()[0] if command.split() else 'cmd')[:20].strip('-')
        log_path = str(Path(log_dir) / f'exec-{ts}-{slug}.log')
        f = open(log_path, 'w', encoding='utf-8', buffering=1)  # line-buffered
        f.write(f'# Command: {command}\n')
        f.write(f'# Time: {time.strftime("%Y-%m-%d %H:%M:%S")}\n\n')
        return f, log_path
    except Exception:
        return None, None


async def execute(input: dict, cwd: str, process_manager=None, log_dir=None) -> dict:
    command = input.get('command', '')
    timeout_ms = min(input.get('timeout', 120000), 600000)
    timeout = timeout_ms / 1000
    background = input.get('background', False)

    # Intercept background process commands — agent can read/list/kill bg processes
    bg_match = re.match(r'^/bg\s*(.*)$', command.strip())
    if bg_match and process_manager:
        return _handle_bg_command(bg_match.group(1).strip(), process_manager)

    for pattern in DANGEROUS_PATTERNS:
        if pattern in command:
            return {'error': f'Blocked dangerous command pattern: {pattern}'}

    # Check sensitive path access
    path_err = check_path_safety(command)
    if path_err:
        return {'error': path_err}

    # Auto-detect likely long-running commands → suggest background
    is_server_like = any(hint in command for hint in BACKGROUND_HINTS)

    # If explicitly background or detected as server-like with a process manager
    if (background or is_server_like) and process_manager:
        bp = await process_manager.launch(command, cwd)
        # Brief wait for early crash detection
        await asyncio.sleep(2.0)
        early_output = '\n'.join(bp.output) if bp.output else '(started, no output yet)'
        if not bp.running:
            result = {
                'output': early_output,
                'exitCode': bp.exit_code,
                'note': f'Process exited (exit {bp.exit_code})',
            }
            if bp.log_path:
                result['logFile'] = bp.log_path
            return result
        result = {
            'output': early_output[:4000],
            'backgrounded': True,
            'processId': bp.id,
            'note': f'Running in background as #{bp.id}. Check output with exec /bg {bp.id} — call it multiple times with sleep in between to watch for startup completion. Log file: {bp.log_path or "N/A"}. Kill: exec /bg kill {bp.id}.',
        }
        if bp.log_path:
            result['logFile'] = bp.log_path
        return result

    log_file, log_path = _open_exec_log(log_dir, command)

    try:
        import sys as _sys
        import time as _time
        _start = _time.time()
        kwargs = {}
        if _sys.platform != 'win32':
            from acorn.background import _unix_preexec
            kwargs['preexec_fn'] = _unix_preexec
        proc = await asyncio.create_subprocess_shell(
            command, cwd=cwd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            **kwargs,
        )

        # Stream stdout line-by-line — inactivity timeout (resets on each line)
        lines = []
        try:
            while True:
                line = await asyncio.wait_for(proc.stdout.readline(), timeout=timeout)
                if not line:
                    break
                decoded = line.decode('utf-8', errors='replace').rstrip('\n')
                lines.append(decoded)
                if log_file:
                    log_file.write(decoded + '\n')
                if on_output:
                    try:
                        on_output(decoded)
                    except Exception:
                        pass
            await proc.wait()
        except asyncio.TimeoutError:
            # Inactivity timeout — no output for `timeout` seconds
            if process_manager:
                bp = await process_manager.launch(command, cwd)
                try:
                    proc.kill()
                except Exception:
                    pass
                if log_file:
                    log_file.write(f'\n# Timed out after {timeout}s of inactivity — moved to background #{bp.id}\n')
                    log_file.close()
                    log_file = None
                return {
                    'output': '\n'.join(lines[-50:]) + f'\n\n[timed out after {timeout}s — moved to background as #{bp.id}]',
                    'backgrounded': True,
                    'processId': bp.id,
                    'logFile': log_path,
                    'note': f'To read latest output: exec /bg {bp.id}. To kill: exec /bg kill {bp.id}.',
                }
            try:
                proc.kill()
            except Exception:
                pass
            if log_file:
                log_file.write(f'\n# Timed out after {timeout}s\n')
                log_file.close()
                log_file = None
            return {'error': f'Command timed out after {timeout}s', 'exitCode': -1, 'logFile': log_path}

        duration_ms = int((_time.time() - _start) * 1000)
        raw_output = '\n'.join(lines)

        # Finalize log
        if log_file:
            log_file.write(f'\n# Exit: {proc.returncode}, Duration: {duration_ms}ms\n')
            log_file.close()
            log_file = None

        output = raw_output
        result = {'output': output, 'exitCode': proc.returncode}
        if len(output) > 8000:
            mid = len(output) - 8000
            output = output[:4000] + f'\n\n[... {mid} chars truncated ...]\n\n' + output[-4000:]
            result['output'] = output
            if log_path:
                result['logFile'] = log_path
                result['note'] = f'Output truncated ({len(raw_output)} chars). Full output: {log_path}'
        elif log_path:
            result['logFile'] = log_path
        return result
    except Exception as e:
        if log_file:
            try:
                log_file.write(f'\n# Error: {e}\n')
                log_file.close()
            except Exception:
                pass
        return {'error': str(e)}
