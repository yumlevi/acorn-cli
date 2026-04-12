"""Terminal output rendering using Rich."""

from rich.console import Console
from rich.live import Live
from rich.markdown import Markdown
from rich.panel import Panel
from rich.syntax import Syntax
from rich.text import Text


class Renderer:
    def __init__(self, console: Console = None):
        self.console = console or Console()
        self._live: None = None
        self._buffer = ''

    def start_streaming(self):
        self._buffer = ''
        self._live = Live(console=self.console, refresh_per_second=8, vertical_overflow='visible')
        self._live.start()

    def stream_delta(self, text: str):
        self._buffer += text
        if self._live:
            try:
                self._live.update(Markdown(self._buffer))
            except Exception:
                self._live.update(Text(self._buffer))

    def finish_streaming(self, usage=None, iterations=None, tool_usage=None):
        if self._live:
            try:
                self._live.update(Markdown(self._buffer))
            except Exception:
                self._live.update(Text(self._buffer))
            self._live.stop()
            self._live = None
        if usage:
            inp = usage.get('input_tokens', 0)
            out = usage.get('output_tokens', 0)
            parts = [f'{inp:,} in', f'{out:,} out']
            if iterations and iterations > 1:
                parts.append(f'{iterations} iters')
            if tool_usage:
                total_tools = sum(tool_usage.values())
                if total_tools:
                    parts.append(f'{total_tools} tools')
            self.console.print(f'[dim]{" · ".join(parts)}[/dim]')
        self.console.print()

    def show_thinking(self, tokens=0):
        if tokens:
            self.console.print(f'[dim italic]  Thinking... ({tokens} tokens)[/dim italic]', end='\r')
        else:
            self.console.print('[dim italic]  Thinking...[/dim italic]', end='\r')

    def clear_thinking(self):
        self.console.print(' ' * 60, end='\r')

    def show_tool_start(self, name: str, detail: str):
        self.console.print(f'  [yellow]⚙ {name}[/yellow] [dim]{detail[:100]}[/dim]')

    def show_tool_done(self, name: str, result_chars: int = 0, duration_ms: int = 0):
        parts = []
        if duration_ms:
            parts.append(f'{duration_ms}ms')
        if result_chars:
            parts.append(f'{result_chars:,} chars')
        self.console.print(f'  [green]✓[/green] [dim]{" · ".join(parts)}[/dim]')

    def show_diff(self, path: str, old_text: str, new_text: str):
        self.console.print(Panel(
            Text.assemble(
                ('- ', 'red'), (old_text[:500], 'red'), '\n',
                ('+ ', 'green'), (new_text[:500], 'green'),
            ),
            title=f'[bold]edit: {path}[/bold]',
            border_style='yellow',
        ))

    def show_code_view(self, path: str, content: str, language: str = 'text', is_new: bool = False):
        label = 'new' if is_new else 'read'
        try:
            syntax = Syntax(content[:5000], language, line_numbers=True, theme='monokai')
            self.console.print(Panel(syntax, title=f'{label}: {path}', border_style='blue'))
        except Exception:
            self.console.print(Panel(content[:2000], title=f'{label}: {path}', border_style='blue'))

    def show_error(self, msg: str):
        self.console.print(f'[bold red]Error:[/bold red] {msg}')

    def show_info(self, msg: str):
        self.console.print(f'[dim]{msg}[/dim]')
