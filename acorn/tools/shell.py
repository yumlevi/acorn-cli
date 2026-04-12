"""Local shell command execution."""

import asyncio


DANGEROUS_PATTERNS = [
    'rm -rf /', 'mkfs', '> /dev/sd', ':(){:|:&};:', 'chmod -R 777 /',
]


async def execute(input: dict, cwd: str) -> dict:
    command = input.get('command', '')
    timeout_ms = min(input.get('timeout', 120000), 600000)
    timeout = timeout_ms / 1000

    for pattern in DANGEROUS_PATTERNS:
        if pattern in command:
            return {'error': f'Blocked dangerous command pattern: {pattern}'}

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
        try:
            proc.kill()
        except Exception:
            pass
        return {'error': f'Command timed out after {timeout}s', 'exitCode': -1}
    except Exception as e:
        return {'error': str(e)}
