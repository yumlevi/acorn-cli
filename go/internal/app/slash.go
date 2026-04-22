package app

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// slashSuggest holds the autocomplete state for the `/` command dropdown.
type slashSuggest struct {
	visible bool
	matches []slashEntry
	cursor  int
}

type slashEntry struct {
	cmd  string
	desc string
}

// All known commands + one-liner descriptions. Extend with the same order
// SlashHelp renders so the picker and /help stay in sync.
var slashCatalog = []slashEntry{
	{"/help", "show this list"},
	{"/new", "start a fresh session in this cwd"},
	{"/clear", "clear chat history"},
	{"/resume", "resume a specific session"},
	{"/sessions", "list saved sessions for this project"},
	{"/quit", "exit"},
	{"/stop", "stop the current generation"},
	{"/plan", "toggle plan/execute mode (same as Shift+Tab)"},
	{"/status", "connection + session info"},
	{"/theme", "switch theme (dark/oak/forest/oled/light/…)"},
	{"/mode", "tool approval mode (auto/ask/locked/yolo/rules)"},
	{"/approve-all", "shortcut for /mode auto"},
	{"/approve-all-dangerous", "shortcut for /mode yolo"},
	{"/bg", "background process list / run / kill"},
	{"/update", "check/install the latest release"},
}

// refreshSuggest recomputes matches for the current input buffer. Only
// fires when the buffer starts with `/` — otherwise we clear suggestions.
func (m *Model) refreshSuggest() {
	text := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(text, "/") || strings.Contains(text, " ") {
		m.suggest.visible = false
		m.suggest.matches = nil
		m.suggest.cursor = 0
		return
	}
	prefix := text
	out := make([]slashEntry, 0, len(slashCatalog))
	for _, e := range slashCatalog {
		if strings.HasPrefix(e.cmd, prefix) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].cmd) < len(out[j].cmd) })
	m.suggest.matches = out
	m.suggest.visible = len(out) > 0
	if m.suggest.cursor >= len(out) {
		m.suggest.cursor = 0
	}
}

// handleSuggestKey intercepts navigation / accept keys while the dropdown
// is open. Returns true if the key was consumed (don't forward to textarea).
func (m *Model) handleSuggestKey(km tea.KeyMsg) (tea.Cmd, bool) {
	if !m.suggest.visible || len(m.suggest.matches) == 0 {
		return nil, false
	}
	switch km.String() {
	case "up", "shift+tab":
		m.suggest.cursor = (m.suggest.cursor - 1 + len(m.suggest.matches)) % len(m.suggest.matches)
		return nil, true
	case "down", "tab":
		m.suggest.cursor = (m.suggest.cursor + 1) % len(m.suggest.matches)
		return nil, true
	case "enter":
		e := m.suggest.matches[m.suggest.cursor]
		m.input.SetValue(e.cmd + " ")
		// Execute immediately on enter only when user hit the one they wanted:
		// if the buffer was already an exact match (e.g., typed /status + enter),
		// execute; otherwise just complete and let the user add args.
		if strings.TrimSpace(m.input.Value()) == e.cmd {
			m.suggest.visible = false
			m.suggest.matches = nil
			return nil, false // fall through to updateKey enter → handleSlashCommand
		}
		m.suggest.visible = false
		return nil, true
	case "esc":
		m.suggest.visible = false
		m.suggest.matches = nil
		return nil, true
	}
	return nil, false
}

// renderSuggest draws the dropdown above the input bar. width is the chat
// column width. Empty string if nothing to show.
func (m *Model) renderSuggest(width int) string {
	if !m.suggest.visible || len(m.suggest.matches) == 0 {
		return ""
	}
	max := 6
	if max > len(m.suggest.matches) {
		max = len(m.suggest.matches)
	}
	start := 0
	if m.suggest.cursor >= max {
		start = m.suggest.cursor - max + 1
	}
	var lines []string
	for i := start; i < start+max && i < len(m.suggest.matches); i++ {
		e := m.suggest.matches[i]
		cmd := e.cmd
		desc := e.desc
		if len(cmd) > 24 {
			cmd = cmd[:23] + "…"
		}
		row := cmd
		if desc != "" {
			padW := 24 - len(cmd)
			if padW < 1 {
				padW = 1
			}
			row += strings.Repeat(" ", padW) + lipgloss.NewStyle().Foreground(m.theme.Muted).Render(desc)
		}
		if i == m.suggest.cursor {
			row = lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true).Render("▸ " + row)
		} else {
			row = "  " + row
		}
		lines = append(lines, row)
	}
	if len(m.suggest.matches) > max {
		lines = append(lines, lipgloss.NewStyle().Foreground(m.theme.Muted).Render(
			"  (+"+itoa(len(m.suggest.matches)-max)+" more — ↓ to scroll)"))
	}
	return borderStyle.Copy().
		BorderForeground(m.theme.Accent2).
		Foreground(m.theme.Fg).
		Padding(0, 1).
		Width(width - 2).
		Render(strings.Join(lines, "\n"))
}
