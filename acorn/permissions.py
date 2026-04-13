"""Permission system — three modes with session-scoped allow rules."""

import re

# Tools that never need approval
ALWAYS_SAFE = {'read_file', 'glob', 'grep'}

# Patterns that ALWAYS require approval even in auto mode
DANGEROUS_PATTERNS = [
    r'\brm\s+(-rf?|--recursive)', r'\brm\s+/', r'rmdir\s+/',
    r'\bmkfs\b', r'>\s*/dev/', r'dd\s+if=',
    r'chmod\s+(-R\s+)?777', r'chown\s+-R\s+.*/',
    r'\bgit\s+push\s+.*--force', r'\bgit\s+reset\s+--hard',
    r'\bdrop\s+table\b', r'\bdrop\s+database\b',
    r'\btruncate\s+table\b',
    r'\bmkfs\.\w+\b', r'\bfdisk\b', r'\bparted\b',
    r':()\{', r'curl.*\|\s*(ba)?sh', r'wget.*\|\s*(ba)?sh',
    r'\bkill\s+-9\b',
]

DANGEROUS_RE = [re.compile(p, re.IGNORECASE) for p in DANGEROUS_PATTERNS]


def is_dangerous(tool_name: str, input: dict) -> bool:
    """Check if a tool call matches dangerous patterns."""
    if tool_name == 'exec':
        cmd = input.get('command', '')
        return any(r.search(cmd) for r in DANGEROUS_RE)
    if tool_name == 'write_file':
        path = input.get('path', '')
        # Writing to system paths
        if path.startswith('/etc/') or path.startswith('/usr/') or path.startswith('/bin/'):
            return True
    return False


def summarize(tool_name: str, input: dict) -> str:
    """Human-readable summary of a tool call."""
    if tool_name == 'exec':
        return input.get('command', '')[:120]
    if tool_name in ('write_file', 'edit_file', 'read_file'):
        return input.get('path', '')
    if tool_name == 'web_fetch':
        return input.get('url', '')[:100]
    if tool_name == 'web_serve':
        return input.get('dir', input.get('directory', ''))[:80]
    return str(input)[:80]


def make_rule(tool_name: str, input: dict) -> str:
    """Generate a session allow-rule from a tool call.
    e.g. exec:git* for 'git status', write_file:src/* for 'src/app.py'
    """
    if tool_name == 'exec':
        cmd = input.get('command', '').strip()
        first_word = cmd.split()[0] if cmd else ''
        if '/' in first_word:
            first_word = first_word.rsplit('/', 1)[-1]
        return f'exec:{first_word}*' if first_word else 'exec:*'
    if tool_name in ('write_file', 'edit_file'):
        path = input.get('path', '')
        if '/' in path:
            dir_part = path.rsplit('/', 1)[0]
            return f'{tool_name}:{dir_part}/*'
        return f'{tool_name}:*'
    return f'{tool_name}:*'


def matches_rule(rule: str, tool_name: str, input: dict) -> bool:
    """Check if a tool call matches an allow rule."""
    if ':' not in rule:
        return rule == tool_name
    rule_tool, rule_pattern = rule.split(':', 1)
    if rule_tool != tool_name:
        return False
    if rule_pattern == '*':
        return True

    if tool_name == 'exec':
        cmd = input.get('command', '').strip()
        # Pattern like "git*" matches "git status", "git push", etc.
        if rule_pattern.endswith('*'):
            prefix = rule_pattern[:-1]
            return cmd.startswith(prefix) or cmd.split()[0] == prefix.rstrip()
        return cmd.startswith(rule_pattern)
    if tool_name in ('write_file', 'edit_file'):
        path = input.get('path', '')
        if rule_pattern.endswith('/*'):
            dir_prefix = rule_pattern[:-2]
            return path.startswith(dir_prefix + '/')
        return path == rule_pattern
    return False


class TuiPermissions:
    """Permission system for the TUI with three modes:

    - auto: approve everything except dangerous commands
    - ask: prompt for every non-safe tool, with option to add session rules
    - locked: approve nothing that isn't in ALWAYS_SAFE
    """

    MODES = ('ask', 'auto', 'locked')

    def __init__(self, app=None, renderer=None):
        self.app = app
        self.renderer = renderer  # for one-shot mode fallback
        self.mode = 'auto' if app is None else 'ask'
        self.session_rules = set()
        self.approve_all = False

    def is_auto_approved(self, tool_name: str, input: dict) -> bool:
        if tool_name in ALWAYS_SAFE:
            return True
        if self.approve_all or self.mode == 'auto':
            if is_dangerous(tool_name, input):
                return False  # dangerous always asks
            return True
        if self.mode == 'locked':
            return False
        # ask mode — check session rules
        for rule in self.session_rules:
            if matches_rule(rule, tool_name, input):
                return True
        return False

    async def prompt(self, tool_name: str, input: dict) -> bool:
        """Show approval UI. Uses TUI selector if app is available, console prompt otherwise."""
        summary = summarize(tool_name, input)
        rule = make_rule(tool_name, input)

        # One-shot / non-TUI mode — simple console prompt
        if not self.app:
            from rich.prompt import Confirm
            loop = __import__('asyncio').get_event_loop()
            return await loop.run_in_executor(
                None, lambda: Confirm.ask(f'  Allow [bold]{tool_name}[/bold]: {summary}?', default=True)
            )
        dangerous = is_dangerous(tool_name, input)

        from rich.text import Text
        t = self.app.theme_data

        # Show what's being requested
        label = Text()
        label.append(f'  ⚙ {tool_name}: ', style=f'bold {t["accent"]}')
        label.append(summary, style=t['fg'])
        if dangerous:
            label.append('  ⚠ DANGEROUS', style=t['error'])
        self.app._log(label)
        self.app._scroll_bottom()

        # Use the question selector UI
        options = ['✓ Allow', f'✓ Allow all {rule}', '✗ Deny']
        if dangerous:
            # No "allow all" for dangerous commands
            options = ['✓ Allow (once)', '✗ Deny']

        # Set up the question selector
        self.app._pending_questions = [{
            'text': 'Allow this action?',
            'options': options,
            'multi': False,
            'index': 1,
        }]
        self.app._pending_answers = {}
        self.app._pending_notes = {}
        self.app._current_question_idx = 0
        self.app._q_selected = 0
        self.app._q_checked = set()
        self.app._q_noting = False
        self.app._q_open_ended = False
        self.app._q_transitioning = False

        # Create a future that the selector will resolve
        self.app._permission_future = self.app._loop.create_future() if hasattr(self.app, '_loop') else None
        self.app._permission_rule = rule
        self.app._permission_dangerous = dangerous
        self.app._answering_questions = True
        self.app._q_permission_mode = True

        self.app._hide_widget('#user-input')
        self.app._show_widget('#question-selector')
        self.app._render_question_selector()
        try:
            from acorn.app import FocusableStatic
            self.app.query_one('#question-selector', FocusableStatic).focus()
        except Exception:
            pass

        # Wait for the user's choice via asyncio Event
        self.app._permission_event = __import__('asyncio').Event()
        await self.app._permission_event.wait()

        # Get the result
        result = getattr(self.app, '_permission_result', False)
        self.app._q_permission_mode = False
        return result
