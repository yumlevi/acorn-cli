package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// codeViewEntry tracks a single file view/diff event.
type codeViewEntry struct {
	Path    string
	Content string // only for view (truncated preview)
	OldText string
	NewText string
	Text    string // short label like "203 lines, 4124 bytes" or "+12/-3 lines"
	IsDiff  bool
	IsNew   bool
	When    time.Time
}

// pushCodeView records a read_file / write_file hit.
func (m *Model) pushCodeView(path, content string, isNew bool) {
	lineCount := strings.Count(content, "\n") + 1
	e := codeViewEntry{
		Path:    path,
		Content: truncateStr(content, 4000),
		Text:    fmt.Sprintf("%d lines, %d bytes", lineCount, len(content)),
		IsNew:   isNew,
		When:    time.Now(),
	}
	m.codeViews = append(m.codeViews, e)
	if len(m.codeViews) > 50 {
		m.codeViews = m.codeViews[len(m.codeViews)-50:]
	}
}

// pushCodeDiff records an edit_file hit.
func (m *Model) pushCodeDiff(path, oldT, newT string) {
	added := strings.Count(newT, "\n")
	removed := strings.Count(oldT, "\n")
	m.codeViews = append(m.codeViews, codeViewEntry{
		Path:    path,
		OldText: truncateStr(oldT, 2000),
		NewText: truncateStr(newT, 2000),
		Text:    fmt.Sprintf("+%d / -%d lines", added, removed),
		IsDiff:  true,
		When:    time.Now(),
	})
	if len(m.codeViews) > 50 {
		m.codeViews = m.codeViews[len(m.codeViews)-50:]
	}
}

// renderCodePanel returns the compact right-column code panel. Height is
// hard-capped by maxH; extra entries scroll off the top.
func (m *Model) renderCodePanel(width, maxH int) string {
	if len(m.codeViews) == 0 || width < 20 || maxH < 5 {
		return ""
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent).Render("Code activity")
	bodyH := maxH - 4 // border(2) + padding(2)
	if bodyH < 1 {
		bodyH = 1
	}
	// Each entry is 2 lines (path+meta); fit as many as we can from the tail.
	perEntry := 2
	maxEntries := bodyH / perEntry
	if maxEntries < 1 {
		maxEntries = 1
	}
	start := len(m.codeViews) - maxEntries
	if start < 0 {
		start = 0
	}
	var lines []string
	for _, e := range m.codeViews[start:] {
		icon := "📄"
		if e.IsDiff {
			icon = "✏️ "
		} else if e.IsNew {
			icon = "🆕"
		}
		path := e.Path
		maxPathW := width - 4 - 4 // border+padding+icon
		if maxPathW < 8 {
			maxPathW = 8
		}
		if len(path) > maxPathW {
			path = "…" + path[len(path)-maxPathW+1:]
		}
		ts := e.When.Format("15:04:05")
		row1 := icon + " " + path
		row2 := lipgloss.NewStyle().Foreground(m.theme.Muted).
			Render("   " + ts + "  " + e.Text)
		lines = append(lines, row1, row2)
	}
	more := ""
	if start > 0 {
		more = "\n" + lipgloss.NewStyle().Foreground(m.theme.Muted).Render(
			"  "+itoa(start)+" older hidden — Ctrl+P to expand")
	}
	inner := title + "\n\n" + strings.Join(lines, "\n") + more
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent2).
		Padding(0, 1).
		Width(width - 2).
		Height(maxH - 2).
		Render(inner)
}

// subagent panel — tracks subagent:* ws frames.
type subagentPanel struct {
	Tasks map[string]*subagentState
	Order []string
}

type subagentState struct {
	TaskID  string
	Title   string
	Status  string
	Lines   []string
	Updated time.Time
}

func newSubagentPanel() *subagentPanel {
	return &subagentPanel{Tasks: map[string]*subagentState{}}
}

func (m *Model) handleSubagentFrame(verb string, raw map[string]any) {
	if m.subagents == nil {
		m.subagents = newSubagentPanel()
	}
	id := asString(raw["taskId"], "")
	if id == "" {
		id = asString(raw["id"], "")
	}
	if id == "" {
		return
	}
	st, ok := m.subagents.Tasks[id]
	if !ok {
		st = &subagentState{TaskID: id, Status: "running"}
		m.subagents.Tasks[id] = st
		m.subagents.Order = append(m.subagents.Order, id)
	}
	st.Updated = time.Now()
	switch verb {
	case "start":
		st.Title = asString(raw["task"], asString(raw["title"], ""))
	case "line", "log":
		line := asString(raw["text"], asString(raw["line"], ""))
		if line != "" {
			st.Lines = append(st.Lines, line)
			if len(st.Lines) > 50 {
				st.Lines = st.Lines[len(st.Lines)-50:]
			}
		}
	case "done":
		st.Status = "done"
	case "error":
		st.Status = "error"
	case "cancelled":
		st.Status = "cancelled"
	}
}

func (m *Model) renderSubagentPanel(width, maxH int) string {
	if m.subagents == nil || len(m.subagents.Order) == 0 || width < 20 || maxH < 5 {
		return ""
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent2).Render("Subagents")
	bodyH := maxH - 4
	if bodyH < 1 {
		bodyH = 1
	}
	// Each task: 1 title line + 1 status line = 2 lines. Fit from tail.
	perTask := 2
	maxTasks := bodyH / perTask
	if maxTasks < 1 {
		maxTasks = 1
	}
	start := len(m.subagents.Order) - maxTasks
	if start < 0 {
		start = 0
	}
	var lines []string
	for _, id := range m.subagents.Order[start:] {
		st := m.subagents.Tasks[id]
		icon := "●"
		color := m.theme.Accent
		switch st.Status {
		case "done":
			icon = "✓"
			color = m.theme.Success
		case "error":
			icon = "✗"
			color = m.theme.Error
		case "cancelled":
			icon = "✕"
			color = m.theme.Muted
		}
		label := trimTo(st.Title, width-8)
		if label == "" {
			label = "(running)"
		}
		tag := lipgloss.NewStyle().Bold(true).Foreground(color).Render(icon + " " + shortID(id))
		lines = append(lines, tag, lipgloss.NewStyle().Foreground(m.theme.Muted).Render("   "+label))
	}
	more := ""
	if start > 0 {
		more = "\n" + lipgloss.NewStyle().Foreground(m.theme.Muted).Render(
			"  "+itoa(start)+" older hidden — Ctrl+P to expand")
	}
	inner := title + "\n\n" + strings.Join(lines, "\n") + more
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent2).
		Padding(0, 1).
		Width(width - 2).
		Height(maxH - 2).
		Render(inner)
}

// renderExpandedPanel — full-screen activity browser (Ctrl+P view) with
// its own scrollable viewport + scrollbar. Up/down/pgup/pgdn scrolls.
func (m *Model) renderExpandedPanel() string {
	body := m.buildExpandedPanelBody()

	// Inner box dimensions after border + padding.
	innerW := m.width - 6 // 2 border + 4 horizontal padding
	if innerW < 10 {
		innerW = 10
	}
	innerH := m.height - 6 // 2 border + 2 vertical padding + 2 hint
	if innerH < 5 {
		innerH = 5
	}

	// Re-init viewport on first open or resize.
	if !m.panelViewInit || m.panelView.Width != innerW || m.panelView.Height != innerH {
		m.panelView = viewport.New(innerW, innerH)
		m.panelViewInit = true
	}
	m.panelView.SetContent(body)

	bar := scrollbar(&m.panelView, innerH, m.theme)
	content := lipgloss.JoinHorizontal(lipgloss.Top, m.panelView.View(), bar)

	hint := lipgloss.NewStyle().Foreground(m.theme.Muted).
		Render("↑↓/PgUp/PgDn scroll · Ctrl+P or Esc to close")

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Accent).
		Padding(1, 2).
		Width(m.width - 2).
		Height(m.height - 2).
		Render(content + "\n" + hint)
}

// buildExpandedPanelBody composes the content string — separate function
// so the viewport can re-content when data changes.
func (m *Model) buildExpandedPanelBody() string {
	var sections []string
	codeTitle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent).Render(
		fmt.Sprintf("Code activity (%d events)", len(m.codeViews)))
	sections = append(sections, codeTitle)
	if len(m.codeViews) == 0 {
		sections = append(sections, lipgloss.NewStyle().Faint(true).Render("  (none yet)"))
	} else {
		for _, e := range m.codeViews {
			icon := "📄"
			if e.IsDiff {
				icon = "✏️ "
			} else if e.IsNew {
				icon = "🆕"
			}
			sections = append(sections, fmt.Sprintf("  %s %s · %s · %s",
				icon, e.Path, e.When.Format("15:04:05"), e.Text))
		}
	}
	sections = append(sections, "")
	saCount := 0
	if m.subagents != nil {
		saCount = len(m.subagents.Order)
	}
	saTitle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent2).Render(
		fmt.Sprintf("Subagents (%d tasks)", saCount))
	sections = append(sections, saTitle)
	if saCount == 0 {
		sections = append(sections, lipgloss.NewStyle().Faint(true).Render("  (none yet)"))
	} else {
		for _, id := range m.subagents.Order {
			st := m.subagents.Tasks[id]
			icon := "●"
			switch st.Status {
			case "done":
				icon = "✓"
			case "error":
				icon = "✗"
			case "cancelled":
				icon = "✕"
			}
			sections = append(sections, fmt.Sprintf("  %s %s — %s", icon, shortID(id), st.Title))
			for _, line := range st.Lines {
				sections = append(sections, lipgloss.NewStyle().Faint(true).Render("    "+trimTo(line, m.width-8)))
			}
		}
	}
	return strings.Join(sections, "\n")
}

func shortID(id string) string {
	if i := strings.LastIndex(id, "_"); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	return id
}

func trimTo(s string, n int) string {
	if n <= 0 {
		return s
	}
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func tailN(xs []string, n int) []string {
	if len(xs) <= n {
		return xs
	}
	return xs[len(xs)-n:]
}

func asString(v any, def string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return def
}
