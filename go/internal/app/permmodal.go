package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// permissionModal is the per-tool approval modal. Shown when the permissions
// layer wants the user to allow/deny a tool call.
type permissionModal struct {
	name      string
	summary   string
	rule      string
	dangerous bool
	selected  int
}

// Options shown. For dangerous tools we only offer Allow / Deny to
// discourage "allow similar" for destructive actions.
func (pm *permissionModal) options() []string {
	if pm.dangerous {
		return []string{"✓ Allow once", "✕ Deny"}
	}
	return []string{"✓ Allow once", "✓ Allow similar (" + pm.rule + ")", "✕ Deny"}
}

func (pm *permissionModal) view(w, h int, t Theme) string {
	title := t.accent(true).Render("Tool approval")
	if pm.dangerous {
		title = lipgloss.NewStyle().Foreground(t.Error).Bold(true).Render("⚠ Dangerous tool approval")
	}
	info := lipgloss.NewStyle().Foreground(t.Fg).Render(pm.name + ": " + pm.summary)
	opts := pm.options()
	var b strings.Builder
	b.WriteString(title + "\n\n" + info + "\n\n")
	for i, o := range opts {
		if i == pm.selected {
			b.WriteString(t.accent(true).Render(" ▸ " + o))
		} else {
			b.WriteString("   " + o)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + t.muted().Render(" ↑↓ select · enter confirm · esc deny"))
	boxW := w - 10
	if boxW < 40 {
		boxW = w - 4
	}
	box := borderStyle.Copy().
		BorderForeground(t.Accent).
		Width(boxW).
		Padding(1, 2).
		Render(b.String())
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) updatePermModal(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	pm := m.permission
	if pm == nil {
		m.modal = modalNone
		return m, nil
	}
	opts := pm.options()
	switch km.String() {
	case "esc":
		m.perms.resolvePerm(false, false)
		m.modal = modalNone
		m.permission = nil
		m.pushChat("system", "Tool denied: "+pm.name)
		return m, nil
	case "up":
		pm.selected = (pm.selected - 1 + len(opts)) % len(opts)
		return m, nil
	case "down":
		pm.selected = (pm.selected + 1) % len(opts)
		return m, nil
	case "enter":
		if pm.dangerous {
			if pm.selected == 0 {
				m.perms.resolvePerm(true, false)
				m.pushChat("system", "Allowed once: "+pm.name)
			} else {
				m.perms.resolvePerm(false, false)
				m.pushChat("system", "Denied: "+pm.name)
			}
		} else {
			switch pm.selected {
			case 0:
				m.perms.resolvePerm(true, false)
				m.pushChat("system", "Allowed once: "+pm.name)
			case 1:
				m.perms.resolvePerm(true, true)
				m.pushChat("system", "Allowed + rule added: "+pm.rule)
			case 2:
				m.perms.resolvePerm(false, false)
				m.pushChat("system", "Denied: "+pm.name)
			}
		}
		m.modal = modalNone
		m.permission = nil
		return m, nil
	}
	return m, nil
}

// Theme style helpers used by modals.
func (t Theme) accent(bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(t.Accent)
	if bold {
		s = s.Bold(true)
	}
	return s
}

func (t Theme) muted() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Muted).Faint(true)
}
