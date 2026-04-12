"""Gather local project context to send with messages."""

import os
import subprocess
from acorn.session import find_git_root


def _git(cmd: str, cwd: str) -> str:
    try:
        r = subprocess.run(
            f'git {cmd}', shell=True, cwd=cwd,
            capture_output=True, text=True, timeout=5,
        )
        return r.stdout.strip() if r.returncode == 0 else ''
    except Exception:
        return ''


def _tree(root: str, max_depth: int = 2, max_entries: int = 50) -> str:
    entries = []
    for dirpath, dirnames, filenames in os.walk(root):
        depth = dirpath.replace(root, '').count(os.sep)
        if depth >= max_depth:
            dirnames.clear()
            continue
        # Skip hidden dirs and common noise
        dirnames[:] = [d for d in sorted(dirnames)
                       if not d.startswith('.') and d not in ('node_modules', '__pycache__', '.git', 'venv', '.venv')]
        indent = '  ' * depth
        basename = os.path.basename(dirpath) or os.path.basename(root)
        entries.append(f'{indent}{basename}/')
        for f in sorted(filenames)[:20]:
            if not f.startswith('.'):
                entries.append(f'{indent}  {f}')
        if len(entries) >= max_entries:
            entries.append(f'{indent}  ... (truncated)')
            break
    return '\n'.join(entries)


def gather_context(cwd: str) -> str:
    git_root = find_git_root(cwd)
    project = os.path.basename(git_root or cwd)
    parts = [f'[Acorn Context — {project}]']
    parts.append(f'CWD: {cwd}')

    if git_root:
        branch = _git('branch --show-current', git_root)
        status = _git('status --short', git_root)
        log = _git('log --oneline -5', git_root)
        parts.append(f'Git: branch={branch}')
        if status:
            lines = status.split('\n')
            if len(lines) > 20:
                status = '\n'.join(lines[:20]) + f'\n... ({len(lines) - 20} more)'
            parts.append(f'Status:\n{status}')
        if log:
            parts.append(f'Recent commits:\n{log}')

    acorn_md = os.path.join(git_root or cwd, 'ACORN.md')
    if os.path.exists(acorn_md):
        try:
            content = open(acorn_md).read()[:4000]
            parts.append(f'--- ACORN.md ---\n{content}\n--- end ---')
        except Exception:
            pass

    tree = _tree(git_root or cwd, max_depth=2, max_entries=50)
    if tree:
        parts.append(f'Project tree:\n{tree}')

    return '\n\n'.join(parts)
