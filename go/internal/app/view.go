package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	userStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5b8af5"))
	botStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#c8cdd8"))
	sysStyle  = lipgloss.NewStyle().Faint(true).Italic(true).Foreground(lipgloss.Color("#7a8595"))
	mutedStyle = lipgloss.NewStyle().Faint(true)
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b6cf7"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#1e2133"))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#e2e6f0")).
			Background(lipgloss.Color("#0e1017")).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7a8595")).
			Background(lipgloss.Color("#0e1017")).
			Padding(0, 1)
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "starting up…"
	}
	header := m.renderHeader()
	body := m.viewport.View()
	input := borderStyle.Width(m.width - 2).Render(m.input.View())
	footer := m.renderFooter()

	main := lipgloss.JoinVertical(lipgloss.Left,
		header,
		body,
		input,
		footer,
	)

	switch m.modal {
	case modalQuestion:
		return overlay(main, m.question.view(m.width, m.height))
	case modalPlan:
		return overlay(main, m.planApproval.view(m.width, m.height))
	}
	return main
}

func (m *Model) renderHeader() string {
	mode := "EXEC"
	if m.planMode {
		mode = "PLAN"
	}
	connIcon := "●"
	if !m.connected {
		connIcon = "○"
	}
	left := fmt.Sprintf("%s acorn · %s · %s", connIcon, m.cfg.User, m.sess)
	right := fmt.Sprintf("[%s]", mode)
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if pad < 1 {
		pad = 1
	}
	return headerStyle.Width(m.width).Render(left + strings.Repeat(" ", pad) + right)
}

func (m *Model) renderFooter() string {
	status := m.status
	if status == "" {
		status = "enter: send · shift+tab: mode · pgup/pgdn: scroll · /help"
	}
	return footerStyle.Width(m.width).Render(status)
}

// layout recomputes viewport + input dimensions for the current window size.
func (m *Model) layout() {
	// Header + input + footer = ~3 + 5 + 1 lines-ish. Give the rest to viewport.
	m.input.SetWidth(m.width - 2)
	inputH := m.input.Height() + 2 // border top/bottom
	m.viewport.Width = m.width
	m.viewport.Height = m.height - 1 /*header*/ - inputH - 1 /*footer*/
	if m.viewport.Height < 3 {
		m.viewport.Height = 3
	}
	m.rerenderViewport()
}

// rerenderViewport formats all messages into a single string and pushes it
// into the viewport. Also scrolls to bottom.
func (m *Model) rerenderViewport() {
	var b strings.Builder
	for _, msg := range m.messages {
		b.WriteString(renderMessage(msg, m.width))
		b.WriteString("\n")
	}
	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}

func renderMessage(c chatMsg, width int) string {
	if c.Role == "system" {
		return sysStyle.Render("  " + c.Text)
	}
	var head, body lipgloss.Style
	switch c.Role {
	case "user":
		head = userStyle
		body = userStyle.Copy().Bold(false)
	case "assistant":
		head = accentStyle
		body = botStyle
	default:
		head = mutedStyle
		body = mutedStyle
	}
	label := strings.ToUpper(c.Role)
	trail := ""
	if c.Streaming {
		trail = accentStyle.Render(" ▌")
	}
	return head.Render(label+":") + "\n" + body.Width(width-2).Render(c.Text) + trail
}

// overlay centres a modal string atop the main view. Bubble Tea doesn't have
// a native z-layer, so we just replace the tail of the frame with the modal.
// Good-enough UX for now; the real thing would use a ModalScreen-style layer
// once Bubble Tea gains one (or via a custom Program with multiple viewports).
func overlay(main, modal string) string {
	// Simplest: render the modal in place of the main frame when open.
	// A proper overlay would require knowing the main dimensions and
	// compositing — skip for MVP.
	return modal
}
