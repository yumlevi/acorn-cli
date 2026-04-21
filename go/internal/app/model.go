package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yumlevi/acorn-cli/go/internal/config"
	"github.com/yumlevi/acorn-cli/go/internal/conn"
	"github.com/yumlevi/acorn-cli/go/internal/proto"
	"github.com/yumlevi/acorn-cli/go/internal/tools"
)

type modalKind int

const (
	modalNone modalKind = iota
	modalQuestion
	modalPlan
	modalPermission
)

// chatMsg is one panel in the chat log.
type chatMsg struct {
	Role      string // "user" | "assistant" | "system" | "tool"
	Text      string
	Timestamp time.Time
	Streaming bool
}

// Model is the Bubble Tea Model.
type Model struct {
	cfg  *config.Config
	cwd  string
	sess string

	client  *conn.Client
	exec    *tools.Executor
	perms   *TUIPerms

	connected bool
	connErr   string

	messages      []chatMsg
	currentStream *chatMsg
	viewport      viewport.Model
	input         textarea.Model
	width, height int

	planMode     bool
	contextSent  bool // gather_context only on first message
	generating   bool
	status       string
	theme        Theme

	// Modals
	modal        modalKind
	question     *questionModal
	planApproval *planModal
	permission   *permissionModal

	// Plan text stashed by ws_events while processing QUESTIONS: in plan mode
	// — same composition fix as the Python 277fc8c commit.
	stashedPlan string

	// sendProgramMsg is wired by main.go after tea.NewProgram so off-thread
	// goroutines (the permissions blocking prompt) can poke the UI.
	sendProgramMsg func(msg tea.Msg)
}

// SetProgram stores the reference so off-thread code can deliver messages.
// main.go calls this after tea.NewProgram returns.
func (m *Model) SetProgram(p *tea.Program) {
	m.sendProgramMsg = func(msg tea.Msg) { p.Send(msg) }
}

// New constructs the initial model.
func New(cfg *config.Config, cwd, sess string, planMode bool) *Model {
	ta := textarea.New()
	ta.Placeholder = "type a message · /help for commands · Shift+Tab toggles plan mode"
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()

	vp := viewport.New(0, 0)
	vp.SetContent("")

	m := &Model{
		cfg:      cfg,
		cwd:      cwd,
		sess:     sess,
		input:    ta,
		viewport: vp,
		planMode: planMode,
		status:   "connecting…",
		theme:    themeForName(cfg.Display.Theme),
	}
	m.perms = newTUIPerms(m)
	m.exec = tools.New(m.perms, cwd, filepath.Join(cwd, ".acorn", "logs"))
	m.exec.Hooks.OnExecLine = func(line string) {
		// Keep a low-noise preview in the status bar.
		if len(line) > 120 {
			line = line[:120] + "…"
		}
		m.status = "⚙ " + line
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	m.client = conn.New(m.cfg.Connection.Host, m.cfg.Connection.Port, m.cfg.Connection.User, m.cfg.Connection.Key)
	m.client.OnConnected = func() { /* handled via connOpenMsg below */ }
	m.client.OnDisconnected = func() { /* likewise */ }
	return tea.Batch(
		m.dialCmd(),
		m.recvCmd(),
		m.toolCmd(),
		textarea.Blink,
	)
}

// dialCmd runs authenticate + connect off the main tea goroutine.
func (m *Model) dialCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if err := m.client.Authenticate(ctx); err != nil {
			return connErrorMsg{err: err.Error()}
		}
		if err := m.client.Connect(ctx); err != nil {
			return connErrorMsg{err: err.Error()}
		}
		return connOpenMsg{}
	}
}

// recvCmd reads a single WS frame.
func (m *Model) recvCmd() tea.Cmd {
	return func() tea.Msg {
		f, ok := <-m.client.In
		if !ok {
			return connClosedMsg{}
		}
		return wsFrameMsg{frame: f}
	}
}

// toolCmd reads a single tool:request frame and executes it.
func (m *Model) toolCmd() tea.Cmd {
	return func() tea.Msg {
		f, ok := <-m.client.ToolRequests
		if !ok {
			return nil
		}
		var req proto.ToolRequest
		if err := json.Unmarshal(f.Raw, &req); err != nil {
			return nil
		}
		// Ack immediately.
		_ = m.client.Send(map[string]any{"type": "tool:ack", "id": req.ID})

		result, claimed := m.exec.Execute(req.Name, req.Input)
		if claimed {
			_ = m.client.Send(map[string]any{
				"type":   "tool:result",
				"id":     req.ID,
				"result": result,
			})
		}
		return toolHandledMsg{name: req.Name}
	}
}

// ── internal message types ────────────────────────────────────────────
type connOpenMsg struct{}
type connErrorMsg struct{ err string }
type connClosedMsg struct{}
type wsFrameMsg struct{ frame conn.Frame }
type toolHandledMsg struct{ name string }
type permDecisionMsg struct{ allowed bool }

// ── message log helpers ──────────────────────────────────────────────
func (m *Model) pushChat(role, text string) {
	m.messages = append(m.messages, chatMsg{Role: role, Text: text, Timestamp: time.Now()})
	m.rerenderViewport()
}

func (m *Model) startStream() {
	m.messages = append(m.messages, chatMsg{Role: "assistant", Text: "", Timestamp: time.Now(), Streaming: true})
	m.currentStream = &m.messages[len(m.messages)-1]
}

func (m *Model) appendDelta(t string) {
	if m.currentStream == nil {
		m.startStream()
	}
	m.currentStream.Text += t
	m.rerenderViewport()
}

func (m *Model) endStream() {
	if m.currentStream != nil {
		m.currentStream.Streaming = false
		m.currentStream = nil
	}
	m.rerenderViewport()
}

// sendChat wraps a chat message with session metadata.
func (m *Model) sendChat(content string) tea.Cmd {
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

// dirTag extracts the last path component for a session label.
func dirTag(cwd string) string {
	cwd = strings.TrimRight(cwd, string(filepath.Separator))
	return filepath.Base(cwd)
}

func _fmtBytes(n int) string { return fmt.Sprintf("%d bytes", n) }
