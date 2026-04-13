"""Panel rendering helpers for the Acorn TUI."""

from rich.text import Text
from rich.panel import Panel
from rich.markdown import Markdown


def themed_panel(theme, content, title='', border_style=None, **kwargs):
    """Create a Panel styled with theme colors."""
    if isinstance(content, str):
        content = Text(content, style=theme['fg'])
    return Panel(
        content,
        title=title,
        title_align='left',
        border_style=border_style or theme['border'],
        style=f'on {theme["bg_panel"]}',
        padding=(0, 1),
        **kwargs,
    )


def themed_text(theme, text, style=None):
    """Create a Text with theme foreground as base."""
    return Text(text, style=style or theme['fg'])


def user_panel(theme, text, username):
    """Render a user message panel."""
    return themed_panel(theme, text, title=f'[bold]{username}[/bold]', border_style=theme['prompt_user'])


def bot_panel(theme, response_text):
    """Render a bot response panel."""
    try:
        content = Markdown(response_text)
    except Exception:
        content = Text(response_text, style=theme['fg'])
    return Panel(
        content,
        title='[bold]acorn[/bold]',
        title_align='left',
        border_style=theme['accent'],
        style=f'on {theme["bg_panel"]}',
        padding=(0, 1),
    )


def error_panel(theme, error_text):
    """Render an error panel."""
    return Panel(
        Text(error_text, style=theme['error']),
        title='[bold]Error[/bold]',
        border_style='red',
        style=f'on {theme["bg_panel"]}',
        padding=(0, 1),
    )
