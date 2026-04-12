"""Local file search — glob and grep."""

import fnmatch
import os
import re


def glob_search(input: dict, cwd: str) -> dict:
    pattern = input.get('pattern', '*')
    search_path = input.get('path', cwd) or cwd
    if not os.path.isabs(search_path):
        search_path = os.path.join(cwd, search_path)

    matches = []
    try:
        for root, dirs, files in os.walk(search_path):
            # Skip hidden/noise dirs
            dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ('node_modules', '__pycache__', '.git')]
            for f in files:
                full = os.path.join(root, f)
                rel = os.path.relpath(full, search_path)
                if fnmatch.fnmatch(rel, pattern) or fnmatch.fnmatch(f, pattern):
                    matches.append(rel)
            if len(matches) >= 500:
                break
    except Exception as e:
        return {'error': str(e)}
    return {'matches': matches[:500], 'count': len(matches)}


def grep_search(input: dict, cwd: str) -> dict:
    pattern = input.get('pattern', '')
    search_path = input.get('path', cwd) or cwd
    if not os.path.isabs(search_path):
        search_path = os.path.join(cwd, search_path)
    file_glob = input.get('glob', input.get('type', ''))

    try:
        regex = re.compile(pattern, re.IGNORECASE if input.get('-i') else 0)
    except re.error as e:
        return {'error': f'Invalid regex: {e}'}

    results = []
    try:
        for root, dirs, files in os.walk(search_path):
            dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ('node_modules', '__pycache__', '.git')]
            for f in files:
                if file_glob and not fnmatch.fnmatch(f, file_glob):
                    continue
                full = os.path.join(root, f)
                rel = os.path.relpath(full, search_path)
                try:
                    with open(full, 'r', errors='replace') as fh:
                        for i, line in enumerate(fh, 1):
                            if regex.search(line):
                                results.append({'file': rel, 'line': i, 'text': line.rstrip()[:200]})
                                if len(results) >= 200:
                                    return {'results': results, 'truncated': True}
                except Exception:
                    continue
    except Exception as e:
        return {'error': str(e)}
    return {'results': results, 'count': len(results)}
