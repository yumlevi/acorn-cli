"""Session persistence — write chat history to local JSONL for crash recovery."""

import json
import time
from pathlib import Path
from acorn.config import GLOBAL_DIR

SESSIONS_DIR = GLOBAL_DIR / 'sessions'


class SessionWriter:
    """Writes chat history to ~/.acorn/sessions/<id>.jsonl as it happens."""

    def __init__(self, session_id: str):
        SESSIONS_DIR.mkdir(parents=True, exist_ok=True)
        safe_id = session_id.replace(':', '_').replace('@', '_').replace('/', '_')[:80]
        self.path = SESSIONS_DIR / f'{safe_id}.jsonl'
        self._file = open(self.path, 'a', buffering=1)  # line-buffered
        self.message_count = 0

    def _append(self, record: dict):
        record['ts'] = time.time()
        try:
            self._file.write(json.dumps(record) + '\n')
            self.message_count += 1
        except Exception:
            pass

    def write_user(self, text: str):
        self._append({'role': 'user', 'text': text})

    def write_assistant(self, text: str, usage: dict = None, iterations: int = None):
        record = {'role': 'assistant', 'text': text}
        if usage:
            record['usage'] = usage
        if iterations:
            record['iterations'] = iterations
        self._append(record)

    def write_tool(self, name: str, input_data: dict, result, local: bool, duration_ms: int = 0):
        self._append({
            'role': 'tool',
            'name': name,
            'input': json.dumps(input_data)[:500] if input_data else '',
            'result_preview': json.dumps(result)[:500] if result else '',
            'local': local,
            'ms': duration_ms,
        })

    def write_error(self, error: str):
        self._append({'role': 'error', 'text': error})

    def close(self):
        try:
            self._file.close()
        except Exception:
            pass


def load_session(session_id: str) -> list:
    """Load a session's message history from local JSONL."""
    safe_id = session_id.replace(':', '_').replace('@', '_').replace('/', '_')[:80]
    path = SESSIONS_DIR / f'{safe_id}.jsonl'
    if not path.exists():
        return []
    messages = []
    try:
        for line in open(path):
            try:
                messages.append(json.loads(line.strip()))
            except json.JSONDecodeError:
                continue
    except Exception:
        pass
    return messages


def cleanup_old_sessions(keep_days: int = 30):
    """Remove sessions older than keep_days."""
    if not SESSIONS_DIR.exists():
        return 0
    cutoff = time.time() - (keep_days * 86400)
    removed = 0
    for f in SESSIONS_DIR.iterdir():
        if f.suffix == '.jsonl' and f.stat().st_mtime < cutoff:
            f.unlink()
            removed += 1
    return removed
