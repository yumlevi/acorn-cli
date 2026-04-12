"""Parse agent questions and present interactive Q&A flow in the TUI."""

import re
import asyncio
from typing import List, Tuple

from textual.app import ComposeResult
from textual.containers import Vertical
from textual.widgets import Static, Input
from textual.binding import Binding
from textual.screen import ModalScreen
from textual.css.query import NoMatches

from rich.text import Text
from rich.panel import Panel


def parse_questions(text: str) -> list:
    """Parse structured questions from agent response.

    Detects patterns like:
      QUESTIONS:
      1. What framework? [React / Vue / Svelte]
      2. TypeScript? [Yes / No]
      3. Target directory?

    Also detects inline questions without the QUESTIONS: marker if they
    have numbered format with optional bracket choices.

    Returns list of dicts: { 'text': str, 'options': list|None, 'index': int }
    """
    questions = []

    # Look for QUESTIONS: block
    blocks = re.split(r'(?:^|\n)\s*QUESTIONS?\s*:\s*\n', text, flags=re.IGNORECASE)
    if len(blocks) > 1:
        q_text = blocks[-1]
    else:
        # Try to find numbered questions anywhere
        q_text = text

    # Parse numbered questions: "1. question text [option1 / option2]"
    pattern = r'(?:^|\n)\s*(\d+)\.\s+(.+?)(?:\n|$)'
    matches = re.finditer(pattern, q_text)

    for m in matches:
        idx = int(m.group(1))
        raw = m.group(2).strip()

        # Extract options from [opt1 / opt2 / opt3] or (opt1 / opt2)
        options = None
        opt_match = re.search(r'[\[\(]([^\]\)]+)[\]\)]', raw)
        if opt_match:
            opts_str = opt_match.group(1)
            options = [o.strip() for o in opts_str.split('/') if o.strip()]
            # Remove the options bracket from the question text
            question_text = raw[:opt_match.start()].strip().rstrip('?').strip() + '?'
        else:
            question_text = raw.rstrip('?').strip() + '?'

        questions.append({
            'text': question_text,
            'options': options if options and len(options) > 1 else None,
            'index': idx,
        })

    return questions


class QuestionScreen(ModalScreen):
    """Modal screen for answering questions interactively."""

    BINDINGS = [
        Binding('escape', 'cancel', 'Cancel'),
    ]

    CSS = """
    QuestionScreen {
        align: center middle;
    }
    #question-container {
        width: 80%;
        max-width: 90;
        height: auto;
        max-height: 80%;
        background: $surface;
        border: round $accent;
        padding: 1 2;
    }
    #q-title {
        text-style: bold;
        margin-bottom: 1;
    }
    #q-text {
        margin-bottom: 1;
    }
    #q-options {
        height: auto;
        margin-bottom: 1;
    }
    #q-input {
        margin-top: 1;
    }
    #q-hint {
        color: $text-muted;
        margin-top: 1;
    }
    """

    def __init__(self, questions: list, theme_data: dict, **kwargs):
        super().__init__(**kwargs)
        self.questions = questions
        self.theme_data = theme_data
        self.current_idx = 0
        self.answers = {}
        self.selected_option = 0

    def compose(self) -> ComposeResult:
        with Vertical(id='question-container'):
            yield Static('', id='q-title')
            yield Static('', id='q-text')
            yield Static('', id='q-options')
            yield Input(placeholder='Type your answer...', id='q-input')
            yield Static('', id='q-hint')

    def on_mount(self):
        self._render_question()

    def _render_question(self):
        if self.current_idx >= len(self.questions):
            self.dismiss(self.answers)
            return

        q = self.questions[self.current_idx]
        t = self.theme_data
        total = len(self.questions)

        title = Text()
        title.append(f' Question {self.current_idx + 1}/{total} ', style=f'bold {t["accent"]}')

        try:
            self.query_one('#q-title', Static).update(title)
            self.query_one('#q-text', Static).update(
                Text(q['text'], style='bold')
            )
        except NoMatches:
            pass

        if q['options']:
            self._render_options(q['options'])
            try:
                self.query_one('#q-input', Input).display = False
                self.query_one('#q-hint', Static).update(
                    Text('↑↓ select · Enter confirm · Esc cancel', style='dim')
                )
            except NoMatches:
                pass
        else:
            try:
                self.query_one('#q-options', Static).update('')
                inp = self.query_one('#q-input', Input)
                inp.display = True
                inp.value = ''
                inp.focus()
                self.query_one('#q-hint', Static).update(
                    Text('Enter to submit · Esc cancel', style='dim')
                )
            except NoMatches:
                pass

        self.selected_option = 0

    def _render_options(self, options: list):
        t = self.theme_data
        lines = Text()
        for i, opt in enumerate(options):
            if i == self.selected_option:
                lines.append(f'  ▸ {opt}', style=f'bold {t["accent"]}')
            else:
                lines.append(f'    {opt}', style='')
            lines.append('\n')
        try:
            self.query_one('#q-options', Static).update(lines)
        except NoMatches:
            pass

    def on_key(self, event):
        q = self.questions[self.current_idx] if self.current_idx < len(self.questions) else None
        if not q or not q['options']:
            return

        if event.key == 'up':
            self.selected_option = (self.selected_option - 1) % len(q['options'])
            self._render_options(q['options'])
            event.prevent_default()
        elif event.key == 'down':
            self.selected_option = (self.selected_option + 1) % len(q['options'])
            self._render_options(q['options'])
            event.prevent_default()
        elif event.key == 'enter':
            self.answers[self.current_idx] = q['options'][self.selected_option]
            self.current_idx += 1
            self._render_question()
            event.prevent_default()

    async def on_input_submitted(self, event: Input.Submitted):
        text = event.value.strip()
        if not text:
            return
        self.answers[self.current_idx] = text
        self.current_idx += 1
        self._render_question()

    def action_cancel(self):
        self.dismiss(None)


def format_answers(questions: list, answers: dict) -> str:
    """Format collected answers into a message to send back to the agent."""
    lines = ['Here are my answers to your questions:\n']
    for i, q in enumerate(questions):
        answer = answers.get(i, '(skipped)')
        lines.append(f'{i + 1}. {q["text"]}')
        lines.append(f'   → {answer}\n')
    return '\n'.join(lines)
