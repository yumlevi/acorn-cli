"""Parse agent questions and present interactive Q&A flow in the TUI."""

import re
import asyncio

from textual.app import ComposeResult
from textual.containers import Vertical, Horizontal
from textual.widgets import Static, Input
from textual.binding import Binding
from textual.screen import ModalScreen
from textual.css.query import NoMatches

from rich.text import Text
from rich.panel import Panel


def parse_questions(text: str) -> list:
    """Parse structured questions from agent response.

    Formats:
      QUESTIONS:
      1. Single select? [React / Vue / Svelte]
      2. Multi select? {React / Vue / Svelte / Angular}
      3. Open-ended question?

    [...] = single select (radio), {...} = multi select (checkboxes)

    Returns list of dicts:
      { 'text': str, 'options': list|None, 'multi': bool, 'index': int }
    """
    questions = []

    # Look for QUESTIONS: block
    blocks = re.split(r'(?:^|\n)\s*QUESTIONS?\s*:\s*\n', text, flags=re.IGNORECASE)
    q_text = blocks[-1] if len(blocks) > 1 else text

    # Parse numbered questions
    pattern = r'(?:^|\n)\s*(\d+)\.\s+(.+?)(?:\n|$)'
    for m in re.finditer(pattern, q_text):
        idx = int(m.group(1))
        raw = m.group(2).strip()

        options = None
        multi = False

        # Multi-select: {opt1 / opt2 / opt3}
        multi_match = re.search(r'\{([^}]+)\}', raw)
        if multi_match:
            opts_str = multi_match.group(1)
            options = [o.strip() for o in opts_str.split('/') if o.strip()]
            question_text = raw[:multi_match.start()].strip().rstrip('?').strip() + '?'
            multi = True
        else:
            # Single-select: [opt1 / opt2 / opt3]
            single_match = re.search(r'\[([^\]]+)\]', raw)
            if single_match:
                opts_str = single_match.group(1)
                options = [o.strip() for o in opts_str.split('/') if o.strip()]
                question_text = raw[:single_match.start()].strip().rstrip('?').strip() + '?'
            else:
                question_text = raw.rstrip('?').strip() + '?'

        questions.append({
            'text': question_text,
            'options': options if options and len(options) > 1 else None,
            'multi': multi,
            'index': idx,
        })

    return questions


class QuestionScreen(ModalScreen):
    """Modal screen for answering questions interactively.

    Supports:
    - Single select (arrow keys, Enter)
    - Multi select (arrow keys, Space to toggle, Enter to confirm)
    - Open-ended text input
    - Tab on any answer to add context/notes
    """

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
        max-height: 85%;
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
        margin-bottom: 0;
    }
    #q-note-area {
        height: auto;
        margin-top: 0;
    }
    #q-input {
        margin-top: 1;
    }
    #q-note-input {
        margin-top: 0;
    }
    #q-hint {
        margin-top: 1;
    }
    """

    def __init__(self, questions: list, theme_data: dict, **kwargs):
        super().__init__(**kwargs)
        self.questions = questions
        self.theme_data = theme_data
        self.current_idx = 0
        self.answers = {}       # idx → str or list
        self.notes = {}         # idx → str (additional context)
        self.selected_option = 0
        self.checked = set()    # for multi-select: set of option indices
        self._noting = False    # True when the note input is active

    def compose(self) -> ComposeResult:
        with Vertical(id='question-container'):
            yield Static('', id='q-title')
            yield Static('', id='q-text')
            yield Static('', id='q-options')
            yield Static('', id='q-note-area')
            yield Input(placeholder='Type your answer...', id='q-input')
            yield Input(placeholder='Add context/notes (optional)...', id='q-note-input')
            yield Static('', id='q-hint')

    def on_mount(self):
        try:
            self.query_one('#q-note-input', Input).display = False
        except NoMatches:
            pass
        self._render_question()

    def _render_question(self):
        if self.current_idx >= len(self.questions):
            self.dismiss({'answers': self.answers, 'notes': self.notes})
            return

        q = self.questions[self.current_idx]
        t = self.theme_data
        total = len(self.questions)
        self.selected_option = 0
        self.checked = set()
        self._noting = False

        # Title
        title = Text()
        title.append(f' Question {self.current_idx + 1}/{total} ', style=f'bold {t["accent"]}')
        if q.get('multi'):
            title.append(' (multi-select)', style=t.get('muted', 'dim'))

        try:
            self.query_one('#q-title', Static).update(title)
            self.query_one('#q-text', Static).update(Text(q['text'], style='bold'))
            self.query_one('#q-note-area', Static).update('')
        except NoMatches:
            pass

        # Hide note input
        try:
            self.query_one('#q-note-input', Input).display = False
        except NoMatches:
            pass

        if q['options']:
            self._render_options()
            try:
                self.query_one('#q-input', Input).display = False
                if q.get('multi'):
                    self.query_one('#q-hint', Static).update(
                        Text('↑↓ move · Space toggle · Tab add note · Enter confirm · Esc cancel', style='dim')
                    )
                else:
                    self.query_one('#q-hint', Static).update(
                        Text('↑↓ select · Tab add note · Enter confirm · Esc cancel', style='dim')
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
                    Text('Enter submit · Tab add note · Esc cancel', style='dim')
                )
            except NoMatches:
                pass

    def _render_options(self):
        q = self.questions[self.current_idx]
        t = self.theme_data
        is_multi = q.get('multi', False)
        options = q['options']

        lines = Text()
        for i, opt in enumerate(options):
            cursor = '▸' if i == self.selected_option else ' '
            if is_multi:
                check = '◉' if i in self.checked else '○'
                if i == self.selected_option:
                    lines.append(f'  {cursor} {check} {opt}', style=f'bold {t["accent"]}')
                elif i in self.checked:
                    lines.append(f'  {cursor} {check} {opt}', style=t.get('success', 'green'))
                else:
                    lines.append(f'  {cursor} {check} {opt}', style='')
            else:
                if i == self.selected_option:
                    lines.append(f'  {cursor} {opt}', style=f'bold {t["accent"]}')
                else:
                    lines.append(f'    {opt}', style='')
            lines.append('\n')

        try:
            self.query_one('#q-options', Static).update(lines)
        except NoMatches:
            pass

    def _show_note_input(self):
        """Show the note input for adding context to the current answer."""
        self._noting = True
        t = self.theme_data
        try:
            self.query_one('#q-note-area', Static).update(
                Text('  Additional context:', style=t.get('muted', 'dim'))
            )
            note_inp = self.query_one('#q-note-input', Input)
            note_inp.display = True
            note_inp.value = self.notes.get(self.current_idx, '')
            note_inp.focus()
            self.query_one('#q-hint', Static).update(
                Text('Enter to save note · Esc to go back', style='dim')
            )
        except NoMatches:
            pass

    def _hide_note_input(self):
        """Hide note input and return to the question."""
        self._noting = False
        try:
            self.query_one('#q-note-input', Input).display = False
            self.query_one('#q-note-area', Static).update('')
        except NoMatches:
            pass
        # Re-render to restore hint text
        q = self.questions[self.current_idx]
        if q['options']:
            self._render_options()
            try:
                if q.get('multi'):
                    self.query_one('#q-hint', Static).update(
                        Text('↑↓ move · Space toggle · Tab add note · Enter confirm · Esc cancel', style='dim')
                    )
                else:
                    self.query_one('#q-hint', Static).update(
                        Text('↑↓ select · Tab add note · Enter confirm · Esc cancel', style='dim')
                    )
            except NoMatches:
                pass
        else:
            try:
                inp = self.query_one('#q-input', Input)
                inp.focus()
                self.query_one('#q-hint', Static).update(
                    Text('Enter submit · Tab add note · Esc cancel', style='dim')
                )
            except NoMatches:
                pass

    def on_key(self, event):
        # If noting, Esc goes back to question (not cancel modal)
        if self._noting and event.key == 'escape':
            # Save whatever is in the note input
            try:
                note_val = self.query_one('#q-note-input', Input).value.strip()
                if note_val:
                    self.notes[self.current_idx] = note_val
            except NoMatches:
                pass
            self._hide_note_input()
            event.prevent_default()
            event.stop()
            return

        if self._noting:
            return  # Let the Input handle keys

        q = self.questions[self.current_idx] if self.current_idx < len(self.questions) else None
        if not q or not q['options']:
            # Tab for open-ended → show note input
            if event.key == 'tab':
                self._show_note_input()
                event.prevent_default()
            return

        if event.key == 'up':
            self.selected_option = (self.selected_option - 1) % len(q['options'])
            self._render_options()
            event.prevent_default()
        elif event.key == 'down':
            self.selected_option = (self.selected_option + 1) % len(q['options'])
            self._render_options()
            event.prevent_default()
        elif event.key == 'space' and q.get('multi'):
            # Toggle checkbox
            if self.selected_option in self.checked:
                self.checked.discard(self.selected_option)
            else:
                self.checked.add(self.selected_option)
            self._render_options()
            event.prevent_default()
        elif event.key == 'tab':
            self._show_note_input()
            event.prevent_default()
        elif event.key == 'enter':
            if q.get('multi'):
                # Multi: collect all checked options
                selected = [q['options'][i] for i in sorted(self.checked)]
                self.answers[self.current_idx] = selected if selected else ['(none selected)']
            else:
                # Single: take highlighted option
                self.answers[self.current_idx] = q['options'][self.selected_option]
            self.current_idx += 1
            self._render_question()
            event.prevent_default()

    async def on_input_submitted(self, event: Input.Submitted):
        input_id = event.input.id if hasattr(event, 'input') else ''

        if self._noting or input_id == 'q-note-input':
            # Save note and go back to question
            text = event.value.strip()
            if text:
                self.notes[self.current_idx] = text
            self._hide_note_input()
            return

        # Regular text answer
        text = event.value.strip()
        if not text:
            return
        self.answers[self.current_idx] = text
        self.current_idx += 1
        self._render_question()

    def action_cancel(self):
        if self._noting:
            self._hide_note_input()
        else:
            self.dismiss(None)


def format_answers(questions: list, answers_data) -> str:
    """Format collected answers into a message to send back to the agent.

    answers_data can be:
    - dict with 'answers' and 'notes' keys (new format)
    - plain dict of idx→answer (legacy format)
    """
    if isinstance(answers_data, dict) and 'answers' in answers_data:
        answers = answers_data['answers']
        notes = answers_data.get('notes', {})
    else:
        answers = answers_data
        notes = {}

    lines = ['Here are my answers to your questions:\n']
    for i, q in enumerate(questions):
        answer = answers.get(i, '(skipped)')
        if isinstance(answer, list):
            answer_str = ', '.join(answer)
        else:
            answer_str = str(answer)

        lines.append(f'{i + 1}. {q["text"]}')
        lines.append(f'   → {answer_str}')

        note = notes.get(i)
        if note:
            lines.append(f'   Note: {note}')
        lines.append('')

    return '\n'.join(lines)
