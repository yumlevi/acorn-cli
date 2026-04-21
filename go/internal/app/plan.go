package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type planModal struct {
	text     string // the plan's prose body
	selected int    // 0=execute, 1=revise, 2=cancel
	feedback string // when in revise-feedback entry mode
	noting   bool
}

func (m *Model) openPlanModal(text string) {
	m.modal = modalPlan
	m.planApproval = &planModal{text: text}
	// Relay plan text to companion observers (mobile shows same modal).
	preview := text
	if len(preview) > 2000 {
		preview = preview[:2000]
	}
	m.Broadcast("plan:show-approval", map[string]any{"text": preview})
}

func (pm *planModal) view(w, h int) string {
	choices := []struct{ label, desc string }{
		{"▶ Execute plan", "Save the plan and switch to execute mode"},
		{"✎ Revise with feedback", "Keep planning — agent will revise"},
		{"✕ Cancel", "Discard the plan"},
	}
	var lines []string
	lines = append(lines, accentStyle.Bold(true).Render("Plan Ready"))
	lines = append(lines, "")
	// Show a short preview of the plan text.
	preview := pm.text
	if len(preview) > 1200 {
		preview = preview[:1200] + "\n…(truncated)"
	}
	lines = append(lines, botStyle.Render(preview))
	lines = append(lines, "")
	if pm.noting {
		lines = append(lines,
			"Type feedback below — press Enter to submit, Esc to go back:",
			"",
			borderStyle.Render(pm.feedback+"▌"),
		)
	} else {
		for i, c := range choices {
			cursor := "  "
			label := c.label
			if i == pm.selected {
				cursor = "▸ "
				label = accentStyle.Bold(true).Render(c.label)
			}
			lines = append(lines, cursor+label+mutedStyle.Render("  "+c.desc))
		}
		lines = append(lines, "", mutedStyle.Render(" ↑↓ select · enter confirm · esc cancel"))
	}

	boxW := w - 10
	if boxW < 50 {
		boxW = w - 4
	}
	box := borderStyle.Copy().
		BorderForeground(lipgloss.Color("#8b6cf7")).
		Width(boxW).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#0e1017")))
}

func (m *Model) updatePlanModal(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	pm := m.planApproval
	if pm == nil {
		m.modal = modalNone
		return m, nil
	}

	if pm.noting {
		switch km.Type {
		case tea.KeyEsc:
			pm.noting = false
			pm.feedback = ""
			return m, nil
		case tea.KeyEnter:
			return m.planReviseWithFeedback(strings.TrimSpace(pm.feedback))
		case tea.KeyBackspace:
			if len(pm.feedback) > 0 {
				pm.feedback = pm.feedback[:len(pm.feedback)-1]
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace:
			pm.feedback += km.String()
			return m, nil
		}
		return m, nil
	}

	switch km.String() {
	case "esc":
		m.modal = modalNone
		m.planApproval = nil
		m.Broadcast("plan:decided", map[string]any{"action": "cancel"})
		m.pushChat("system", "Plan dismissed.")
		return m, nil
	case "up":
		pm.selected = (pm.selected - 1 + 3) % 3
		return m, nil
	case "down":
		pm.selected = (pm.selected + 1) % 3
		return m, nil
	case "enter":
		switch pm.selected {
		case 0:
			return m.planExecute(pm.text)
		case 1:
			pm.noting = true
			return m, nil
		case 2:
			m.modal = modalNone
			m.planApproval = nil
			m.Broadcast("plan:decided", map[string]any{"action": "cancel"})
			m.pushChat("system", "Plan discarded.")
			return m, nil
		}
	}
	return m, nil
}

// planExecute saves the plan, flips to execute mode, and sends PLAN_EXECUTE.
func (m *Model) planExecute(text string) (tea.Model, tea.Cmd) {
	if path := savePlan(m.cwd, text); path != "" {
		m.pushChat("system", "Plan saved to "+path)
	} else {
		m.pushChat("system", "Plan save FAILED — check permissions on .acorn/plans/")
	}
	m.planMode = false
	m.modal = modalNone
	m.planApproval = nil
	m.Broadcast("plan:decided", map[string]any{"action": "execute"})
	m.Broadcast("plan:set-mode", map[string]any{"enabled": false})
	m.pushChat("system", "Mode → execute")
	m.pushChat("system", "▶ Executing plan…")
	m.generating = true
	m.status = "waiting…"
	return m, m.sendChatMessage(PlanExecuteMsg)
}

func (m *Model) planReviseWithFeedback(fb string) (tea.Model, tea.Cmd) {
	m.modal = modalNone
	m.planApproval = nil
	if fb == "" {
		m.pushChat("system", "Plan revise cancelled (empty feedback).")
		return m, nil
	}
	m.pushChat("user", "(feedback) "+fb)
	m.generating = true
	m.status = "waiting…"
	m.Broadcast("plan:decided", map[string]any{"action": "revise", "feedback": fb})
	return m, m.sendChatMessage("[PLAN FEEDBACK: Revise the plan. Stay in plan mode.]\n\n" + fb)
}

// sendChatMessage lets plan+ask_user flows send arbitrary content.
func (m *Model) sendChatMessage(content string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.Send(map[string]any{
			"type":      "chat",
			"sessionId": m.sess,
			"content":   content,
			"userName":  m.cfg.Connection.User,
			"cwd":       m.cwd,
		})
		if err != nil {
			return connErrorMsg{err: err.Error()}
		}
		return nil
	}
}

// savePlan mirrors acorn/cli.py:_save_plan — writes the plan to
// {cwd}/.acorn/plans/plan-<ts>.md. Returns empty string on failure.
func savePlan(cwd, text string) string {
	dir := filepath.Join(cwd, ".acorn", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "[plan-save] mkdir:", err)
		return ""
	}
	ts := time.Now().Format("20060102-150405")
	name := "plan-" + ts + ".md"
	full := filepath.Join(dir, name)
	clean := strings.TrimSpace(strings.ReplaceAll(text, "PLAN_READY", ""))
	body := "# Plan — " + ts + "\n\n" + clean + "\n"
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "[plan-save] write:", err)
		return ""
	}
	return full
}
