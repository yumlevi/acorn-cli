package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Module-level styles referenced by the modal files. They're built from
// themeDark at init time — themes that differ from dark still work because
// the modals overlay the entire screen, so only the accent/muted contrast
// matters in practice.
var (
	borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#1e2133"))

	accentStyle = lipgloss.NewStyle().Foreground(themeDark.Accent).Bold(true)
	mutedStyle  = lipgloss.NewStyle().Foreground(themeDark.Muted).Faint(true)
	botStyle    = lipgloss.NewStyle().Foreground(themeDark.BotPanel)
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "starting up…"
	}
	header := m.renderHeader()
	body := m.viewport.View()

	inputStyle := borderStyle.Copy().
		BorderForeground(m.theme.Separator).
		Width(m.width - 2)
	input := inputStyle.Render(m.input.View())
	footer := m.renderFooter()

	main := lipgloss.JoinVertical(lipgloss.Left, header, body, input, footer)

	switch m.modal {
	case modalQuestion:
		return m.question.view(m.width, m.height)
	case modalPlan:
		return m.planApproval.view(m.width, m.height)
	case modalPermission:
		return m.permission.view(m.width, m.height, m.theme)
	}
	return main
}

func (m *Model) renderHeader() string {
	mode := "EXEC"
	modeBg := m.theme.ModeBarExecBg
	if m.planMode {
		mode = "PLAN"
		modeBg = m.theme.ModeBarPlanBg
	}
	connIcon := "●"
	if !m.connected {
		connIcon = "○"
	}
	left := fmt.Sprintf("%s acorn · %s · %s", connIcon, m.cfg.Connection.User, short(m.sess))
	right := fmt.Sprintf(" [%s] ", mode)

	leftStyle := lipgloss.NewStyle().Bold(true).
		Foreground(m.theme.Fg).Background(m.theme.BgPanel).Padding(0, 1)
	rightStyle := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("#ffffff")).Background(modeBg).Padding(0, 1)

	pad := m.width - lipgloss.Width(leftStyle.Render(left)) - lipgloss.Width(rightStyle.Render(right))
	if pad < 0 {
		pad = 0
	}
	fill := lipgloss.NewStyle().Background(m.theme.BgPanel).Render(strings.Repeat(" ", pad))
	return leftStyle.Render(left) + fill + rightStyle.Render(right)
}

func (m *Model) renderFooter() string {
	status := m.status
	if status == "" {
		status = "enter: send · shift+tab: mode · pgup/pgdn: scroll · /help"
	}
	return lipgloss.NewStyle().
		Foreground(m.theme.Muted).
		Background(m.theme.BgPanel).
		Padding(0, 1).
		Width(m.width).
		Render(status)
}

func (m *Model) layout() {
	m.input.SetWidth(m.width - 2)
	inputH := m.input.Height() + 2
	m.viewport.Width = m.width
	m.viewport.Height = m.height - 1 - inputH - 1
	if m.viewport.Height < 3 {
		m.viewport.Height = 3
	}
	m.rerenderViewport()
}

func (m *Model) rerenderViewport() {
	var b strings.Builder
	for _, msg := range m.messages {
		b.WriteString(renderMessage(msg, m.width, m.theme))
		b.WriteString("\n")
	}
	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

func renderMessage(c chatMsg, width int, t Theme) string {
	if c.Role == "system" {
		return lipgloss.NewStyle().Foreground(t.System).Italic(true).Render("  " + c.Text)
	}
	var headColor, bodyColor lipgloss.Color
	var label string
	switch c.Role {
	case "user":
		headColor, bodyColor = t.UserPanel, t.Fg
		label = strings.ToUpper(c.Role)
	case "assistant":
		headColor, bodyColor = t.Accent2, t.BotPanel
		label = strings.ToUpper(c.Role)
	default:
		headColor, bodyColor = t.Muted, t.Muted
		label = strings.ToUpper(c.Role)
	}
	head := lipgloss.NewStyle().Bold(true).Foreground(headColor).Render(label + ":")
	body := lipgloss.NewStyle().Foreground(bodyColor).Width(width - 2).Render(c.Text)
	trail := ""
	if c.Streaming {
		trail = lipgloss.NewStyle().Foreground(t.Accent).Render(" ▌")
	}
	return head + "\n" + body + trail
}

// short tail-truncates a session id for the header.
func short(s string) string {
	if len(s) <= 40 {
		return s
	}
	return "…" + s[len(s)-38:]
}
