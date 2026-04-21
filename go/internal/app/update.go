package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yumlevi/acorn-cli/go/internal/proto"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Modal intercept — keystrokes belong to the modal when one is open.
	if m.modal != modalNone {
		return m.updateModal(msg)
	}

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		return m.updateKey(msg)

	case connOpenMsg:
		m.connected = true
		m.connErr = ""
		m.status = "connected"
		m.pushChat("system", fmt.Sprintf("Connected to %s as %s (session %s)", m.cfg.ServerURL, m.cfg.User, m.sess))
		// Request history for this session
		_ = m.client.Send(proto.Out{Type: "chat:history-request", SessionID: m.sess, UserName: m.cfg.User})
		return m, nil

	case connErrorMsg:
		m.connected = false
		m.connErr = msg.err
		m.pushChat("system", "Connection error: "+msg.err)
		return m, nil

	case connClosedMsg:
		m.connected = false
		m.pushChat("system", "Disconnected.")
		m.status = "disconnected"
		return m, nil

	case wsFrameMsg:
		cmd := m.handleFrame(msg.frame)
		return m, tea.Batch(cmd, m.recvCmd())
	}

	// Forward anything else to the input.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateKey handles keystrokes when no modal is open.
func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+d":
		m.client.Close()
		return m, tea.Quit

	case "shift+tab":
		m.planMode = !m.planMode
		label := "execute"
		if m.planMode {
			label = "plan"
		}
		m.pushChat("system", "Mode → "+label)
		return m, nil

	case "enter":
		// Enter sends; Alt+Enter inserts a newline. textarea handles it.
		if msg.Alt {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return m, nil
		}
		if strings.HasPrefix(text, "/") {
			return m.handleSlashCommand(text)
		}
		m.input.Reset()
		m.pushChat("user", text)
		content := text
		if m.planMode {
			content = "[PLAN MODE]\n" + content
		}
		m.generating = true
		m.status = "waiting…"
		return m, m.sendChat(content)

	case "pgup":
		m.viewport.LineUp(m.viewport.Height - 2)
		return m, nil
	case "pgdown":
		m.viewport.LineDown(m.viewport.Height - 2)
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleSlashCommand intercepts /help, /new, /clear, /resume, /quit.
func (m *Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	m.input.Reset()
	fields := strings.Fields(text)
	cmd := fields[0]
	switch cmd {
	case "/help":
		m.pushChat("system", `/help — this list
/new — start a new session in this cwd
/clear — clear the local view (server-side history unchanged)
/resume <sessionId> — resume a specific session
/quit — exit
Shift+Tab — toggle plan / execute mode
PgUp/PgDn — scroll chat history`)
	case "/clear":
		m.messages = m.messages[:0]
		m.rerenderViewport()
	case "/new":
		m.sess = fmt.Sprintf("cli:%s@%s-%s", m.cfg.User, dirTag(m.cwd), shortID())
		m.messages = m.messages[:0]
		m.rerenderViewport()
		m.pushChat("system", "New session: "+m.sess)
	case "/resume":
		if len(fields) < 2 {
			m.pushChat("system", "Usage: /resume <sessionId>")
			return m, nil
		}
		m.sess = fields[1]
		m.messages = m.messages[:0]
		m.rerenderViewport()
		_ = m.client.Send(proto.Out{Type: "chat:history-request", SessionID: m.sess, UserName: m.cfg.User})
		m.pushChat("system", "Resumed: "+m.sess)
	case "/quit":
		m.client.Close()
		return m, tea.Quit
	default:
		m.pushChat("system", "Unknown command: "+cmd+" (type /help)")
	}
	return m, nil
}

// handleFrame routes a single inbound server frame.
func (m *Model) handleFrame(f proto.In) tea.Cmd {
	switch f.Type {
	case "chat:start":
		m.startStream()
	case "chat:delta":
		m.appendDelta(f.Text)
	case "chat:done":
		m.endStream()
		m.generating = false
		m.status = ""
		// After end-of-turn, inspect the final assistant text for markers
		// the server didn't convert to structured messages (QUESTIONS: /
		// PLAN_READY). This keeps acorn usable against servers that don't
		// yet emit the new ask_user / plan_proposal messages.
		if m.currentStream == nil && len(m.messages) > 0 {
			last := m.messages[len(m.messages)-1]
			if last.Role == "assistant" {
				if qs := parseQuestionsBlock(last.Text); qs != nil {
					m.openQuestionModal(qs)
				} else if m.planMode && strings.Contains(last.Text, "PLAN_READY") {
					m.openPlanModal(last.Text)
				}
			}
		}
	case "chat:thinking":
		// No visible panel for thinking by default — just status.
		m.status = "thinking…"
	case "chat:status":
		m.status = f.Status
	case "chat:tool":
		if f.Tool != "" {
			m.status = fmt.Sprintf("⚙ %s %s", f.Tool, f.Detail)
		}
	case "chat:history":
		for _, h := range f.Messages {
			role := h.Role
			if role != "user" && role != "assistant" {
				role = "system"
			}
			m.messages = append(m.messages, chatMsg{Role: role, Text: h.Text})
		}
		m.rerenderViewport()
	case "chat:error":
		m.pushChat("system", "chat error: "+f.Error)
		m.generating = false
		m.status = ""

	// Structured question from the new ask_user tool on the server.
	case "ask_user":
		m.openStructuredQuestion(f)

	// Tool-queue plan mode (web side — the CLI doesn't use this but we
	// surface it for visibility if the server sends it).
	case "plan_proposal":
		m.pushChat("system", fmt.Sprintf("[plan] queued #%d: %s — %s", f.ProposalID, f.Tool, f.Summary))
	case "plan_applied":
		for _, r := range f.Results {
			mark := "✓"
			if !r.OK {
				mark = "✗"
			}
			m.pushChat("system", fmt.Sprintf("[plan] %s %s %s", mark, r.Tool, r.Summary))
		}
	case "plan_rejected":
		m.pushChat("system", "[plan] proposals rejected")
	case "plan_mode":
		if f.Enabled != nil {
			m.planMode = *f.Enabled
			label := "execute"
			if m.planMode {
				label = "plan"
			}
			m.pushChat("system", "Mode → "+label+" (remote)")
		}

	case "conn:error":
		m.pushChat("system", "[ws] "+f.Error)
	}
	return nil
}
