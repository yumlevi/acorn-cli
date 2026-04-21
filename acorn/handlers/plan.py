"""Plan handler — owns plan state, communicates via bridge."""

import asyncio
import json
import re
from dataclasses import dataclass
from rich.text import Text
from rich.panel import Panel
from rich.table import Table
from textual.css.query import NoMatches

from acorn.constants import PLAN_EXECUTE_MSG
from acorn.protocol import chat_message


@dataclass
class PlanState:
    last_plan_text: str = ''
    awaiting_decision: bool = False
    awaiting_feedback: bool = False


class PlanHandler:
    """Handles plan approval flow. Owns its own state."""

    def __init__(self, bridge):
        self.bridge = bridge
        self.state = PlanState()

    def show_choices(self):
        b = self.bridge
        t = b.theme
        b.log(Text(''))

        # Show file summary
        if self.state.last_plan_text:
            self._show_file_summary(self.state.last_plan_text)

        # Broadcast to companion app so it shows plan approval too
        b.broadcast('plan:show-approval', text=self.state.last_plan_text[:2000])

        # Use questions handler for the selector. IMPORTANT: start_questions
        # replaces the entire QuestionState, so plan_approval must be set
        # AFTER that call — otherwise the flag is wiped and the Execute
        # answer falls through to the normal-answer branch, which re-fires
        # _send_answers and loops indefinitely when a plan is stashed.
        qh = b.get_questions_handler()
        qh.start_questions([{
            'text': 'Plan ready — what would you like to do?',
            'options': ['▶ Execute plan', '✎ Revise with feedback', '✕ Cancel'],
            'multi': False,
            'index': 1,
        }])
        qh.state.plan_approval = True

    def _show_file_summary(self, plan_text):
        b = self.bridge
        t = b.theme
        create_re = re.compile(r'(?:create|new file|write)\s+[`"]?([a-zA-Z0-9_./-]+\.[a-zA-Z0-9]+)', re.IGNORECASE)
        modify_re = re.compile(r'(?:modify|edit|update|change)\s+[`"]?([a-zA-Z0-9_./-]+\.[a-zA-Z0-9]+)', re.IGNORECASE)

        creates = set(m.group(1) for m in create_re.finditer(plan_text))
        modifies = set(m.group(1) for m in modify_re.finditer(plan_text))
        noise = {'e.g.', 'i.e.', 'etc.', 'v1.0', 'v2.0', 'PLAN_READY'}
        creates -= noise
        modifies -= noise

        if creates or modifies:
            table = Table.grid(padding=(0, 2))
            table.add_column(style=t.get('muted', 'dim'), min_width=8)
            table.add_column()
            for f in sorted(creates):
                table.add_row(Text('create', style=t['success']), Text(f, style=t['fg']))
            for f in sorted(modifies - creates):
                table.add_row(Text('modify', style=t['edit_icon']), Text(f, style=t['fg']))
            b.log(Panel(table, title='[bold]Files affected[/bold]', border_style=t['accent'],
                         style=f'on {t["bg_panel"]}', padding=(0, 1)))
            b.scroll_bottom()

    def _broadcast_decision(self, action, feedback=None):
        """Broadcast plan decision to companion app observers."""
        b = self.bridge
        kwargs = {'action': action}
        if feedback:
            kwargs['feedback'] = feedback
        b.broadcast('plan:decided', **kwargs)

    def _broadcast_plan_mode(self, enabled):
        """Broadcast plan mode change to companion app observers."""
        self.bridge.broadcast('plan:set-mode', enabled=enabled)

    def handle_decision(self, text):
        b = self.bridge
        s = self.state
        s.awaiting_decision = False
        b.sm.transition(b.AppState.IDLE)
        t = b.theme

        # If the questions handler queued answers while we were composing
        # the plan approval, retrieve them here so each branch below can
        # decide whether to fire them on the wire.
        qh = b.get_questions_handler()
        pending_answers = getattr(qh, '_pending_answers', None)
        if pending_answers is not None:
            qh._pending_answers = None

        # Clear the stashed plan text on EVERY branch. If we don't, the
        # next user response could re-trigger _send_answers -> show_choices
        # because the stash is still populated and plan_mode may not be
        # reset yet (revise keeps plan_mode on).
        plan_text_for_save = s.last_plan_text
        s.last_plan_text = ''

        if text == '1' or text.lower().startswith('exec'):
            from acorn.cli import _save_plan
            plan_path = _save_plan(b.cwd, plan_text_for_save)
            if plan_path:
                b.log(b.themed_text(f'  Plan saved to {plan_path}', style=t['muted']))

            b.plan_mode = False
            b.log(b.themed_text('  Mode → execute', style=t['accent']))
            b.log(b.themed_text('  ▶ Executing plan...', style=t['success']))
            b.scroll_bottom()

            b.generating = True
            b.update_footer()
            b.update_mode_bar()
            b.update_header()
            # If there were queued answers, send them first so the agent has
            # the user's input in context, THEN fire the execute signal. The
            # two messages arrive in order; the agent's next turn acts on
            # the plan with the answers already absorbed.
            if pending_answers:
                asyncio.create_task(b.conn.send(chat_message(b.session_id, pending_answers, b.user, cwd=b.cwd)))
            asyncio.create_task(b.conn.send(chat_message(b.session_id, PLAN_EXECUTE_MSG, b.user, cwd=b.cwd)))

            # Notify observers
            self._broadcast_decision('execute')
            self._broadcast_plan_mode(False)

        elif text == '3' or text.lower().startswith('cancel'):
            b.log(b.themed_text('  Plan discarded', style=t['muted']))
            b.scroll_bottom()
            # Even on cancel, fire the queued answers if any — the agent
            # should still know what the user chose, even if the plan is
            # being dropped. They may want a different approach entirely.
            if pending_answers:
                asyncio.create_task(b.conn.send(chat_message(b.session_id, pending_answers, b.user, cwd=b.cwd)))
            self._broadcast_decision('cancel')

        else:
            feedback = text if text != '2' else ''
            if text == '2':
                b.log(Text('  Type your feedback:', style=t['muted']))
                s.awaiting_decision = True
                s.awaiting_feedback = True
                b.sm.transition(b.AppState.PLAN_FEEDBACK)
                b.scroll_bottom()
                return

            b.log(b.themed_panel(
                feedback,
                title=f'[bold]{b.user}[/bold] [dim](feedback)[/dim]',
                border_style=t['prompt_user'],
            ))
            b.scroll_bottom()

            feedback_msg = f'[PLAN FEEDBACK: Revise the plan. Stay in plan mode.]\n\n{feedback}'
            b.generating = True
            b.update_footer()
            b.update_header()
            # Send queued answers first so the revision sees them, then the
            # feedback. Agent will revise with both in context.
            if pending_answers:
                asyncio.create_task(b.conn.send(chat_message(b.session_id, pending_answers, b.user, cwd=b.cwd)))
            asyncio.create_task(b.conn.send(chat_message(b.session_id, feedback_msg, b.user, cwd=b.cwd)))

            # Notify observers
            self._broadcast_decision('revise', feedback)
