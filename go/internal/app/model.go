// Package app is the Bubble Tea model for the acorn CLI.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/yumlevi/acorn-cli/go/internal/config"
	"github.com/yumlevi/acorn-cli/go/internal/conn"
	"github.com/yumlevi/acorn-cli/go/internal/proto"
)

// Modal kinds — at most one visible at a time.
type modalKind int

const (
	modalNone modalKind = iota
	modalQuestion
	modalPlan
)

// Chat message log entry.
type chatMsg struct {
	Role      string // "user" | "assistant" | "system" | "tool"
	Text      string
	Timestamp time.Time
	// Set when an assistant message is still receiving deltas. View renders
	// a trailing cursor while true.
	Streaming bool
}

type Model struct {
	cfg  *config.Config
	cwd  string
	sess string

	client    *conn.Client
	connected bool
	connErr   string

	// Chat state.
	messages       []chatMsg
	currentStream  *chatMsg // pointer into messages for the active assistant turn
	viewport       viewport.Model
	input          textarea.Model
	width, height  int

	// Mode + status
	planMode   bool
	generating bool
	status     string // bottom-bar activity string

	// Modals
	modal        modalKind
	question     *questionModal
	planApproval *planModal
}

// New constructs the initial model. Connection happens via Init() as a tea.Cmd.
func New(cfg *config.Config, cwd, sessionID string) *Model {
	if sessionID == "" {
		sessionID = fmt.Sprintf("cli:%s@%s-%s", cfg.User, dirTag(cwd), shortID())
	}

	ta := textarea.New()
	ta.Placeholder = "type a message, /help for commands, Shift+Tab toggles plan mode"
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()

	vp := viewport.New(0, 0)
	vp.SetContent("")

	return &Model{
		cfg:      cfg,
		cwd:      cwd,
		sess:     sessionID,
		input:    ta,
		viewport: vp,
		planMode: cfg.PlanMode,
		status:   "connecting…",
	}
}

func (m *Model) Init() tea.Cmd {
	m.client = conn.New(m.cfg.ServerURL, m.cfg.TeamKey, m.cfg.User)
	return tea.Batch(
		dialCmd(m.client),
		m.recvCmd(),
		textarea.Blink,
	)
}

// dialCmd runs the WS dial off the main tea goroutine.
func dialCmd(c *conn.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if err := c.Dial(ctx); err != nil {
			return connErrorMsg{err: err.Error()}
		}
		return connOpenMsg{}
	}
}

// recvCmd reads a single inbound frame and returns it as a tea.Msg. The
// update loop re-arms recvCmd after each receive so the channel keeps
// pumping frames into the UI.
func (m *Model) recvCmd() tea.Cmd {
	return func() tea.Msg {
		f, ok := <-m.client.In
		if !ok {
			return connClosedMsg{}
		}
		return wsFrameMsg{frame: f}
	}
}

// ── internal message types ────────────────────────────────────────────
type connOpenMsg struct{}
type connErrorMsg struct{ err string }
type connClosedMsg struct{}
type wsFrameMsg struct{ frame proto.In }

// shortID is a 6-char hex from uuid for session ids.
func shortID() string {
	id := uuid.New().String()
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) < 6 {
		return clean
	}
	return clean[:6]
}

// dirTag extracts the last 2 path components for a session label.
func dirTag(cwd string) string {
	cwd = strings.TrimRight(cwd, "/")
	if cwd == "" {
		return "cwd"
	}
	parts := strings.Split(cwd, "/")
	if len(parts) < 2 {
		return parts[len(parts)-1]
	}
	return parts[len(parts)-1]
}

// pushChat appends a completed message to the history and re-renders.
func (m *Model) pushChat(role, text string) {
	m.messages = append(m.messages, chatMsg{Role: role, Text: text, Timestamp: time.Now()})
	m.rerenderViewport()
}

// startStream starts a new streaming assistant message. Subsequent deltas
// append to its Text field.
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

// sendChat wraps proto.Out Send with the session's metadata filled.
func (m *Model) sendChat(content string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.Send(proto.Out{
			Type:      "chat",
			SessionID: m.sess,
			Content:   content,
			UserName:  m.cfg.User,
			CWD:       m.cwd,
		})
		if err != nil {
			return connErrorMsg{err: err.Error()}
		}
		return nil
	}
}
