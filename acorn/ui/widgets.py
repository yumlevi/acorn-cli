"""Custom Textual widgets for the Acorn TUI."""

from textual.widgets import Static, RichLog, TextArea


class MessageInput(TextArea):
    """TextArea that submits on Enter and inserts newline on Ctrl+J.
    Also handles autocomplete navigation when the popup is visible."""

    class Submitted:
        """Fired when user presses Enter to submit."""
        def __init__(self, text):
            self.text = text

    def on_key(self, event):
        app = self.app

        # If autocomplete is showing, route keys to it
        if getattr(app, '_autocomplete_matches', []):
            if event.key == 'enter' or event.key == 'tab':
                idx = getattr(app, '_autocomplete_selected', 0)
                matches = app._autocomplete_matches
                if idx < len(matches):
                    cmd, _ = matches[idx]
                    self.clear()
                    self.insert(cmd + ' ')
                app._autocomplete_matches = []
                app._hide_widget('#autocomplete')
                event.prevent_default()
                event.stop()
                return
            elif event.key == 'up':
                app._autocomplete_selected = (app._autocomplete_selected - 1) % min(len(app._autocomplete_matches), 8)
                app._render_autocomplete()
                event.prevent_default()
                event.stop()
                return
            elif event.key == 'down':
                app._autocomplete_selected = (app._autocomplete_selected + 1) % min(len(app._autocomplete_matches), 8)
                app._render_autocomplete()
                event.prevent_default()
                event.stop()
                return
            elif event.key == 'escape':
                app._autocomplete_matches = []
                app._hide_widget('#autocomplete')
                event.prevent_default()
                event.stop()
                return

        # Enter → send message
        if event.key == 'enter':
            text = self.text.strip()
            if text:
                if hasattr(app, 'on_message_input_submitted'):
                    app.on_message_input_submitted(self.Submitted(text))
                self.clear()
            event.prevent_default()
            event.stop()
            return

        # Ctrl+J → insert newline
        if event.key == 'ctrl+j':
            self.insert('\n')
            event.prevent_default()
            event.stop()
            return


class FocusableStatic(Static):
    """A Static widget that can receive focus for key events.
    Routes key events to the app's question handler when active."""
    can_focus = True

    def on_key(self, event):
        app = self.app
        if not getattr(app, '_answering_questions', False):
            return
        if getattr(app, '_q_transitioning', False):
            event.prevent_default()
            event.stop()
            return
        if event.key in ('up', 'down', 'space', 'tab', 'enter', 'escape'):
            app._handle_question_key(event.key)
            event.prevent_default()
            event.stop()


class SelectableLog(RichLog):
    """RichLog that doesn't capture mouse click/drag, allowing terminal-native text selection.
    Mouse scroll is preserved for scrolling through conversation."""

    def _on_mouse_down(self, event):
        pass  # Let terminal handle click/drag for text selection

    def _on_mouse_up(self, event):
        pass

    # Scroll events are inherited from RichLog and still work
