package app

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yumlevi/acorn-cli/go/internal/conn"
	"github.com/yumlevi/acorn-cli/go/internal/proto"
)

// PlanPrefix — port of acorn/constants.py:PLAN_PREFIX. Prepended to the
// user's first plan-mode message.
const PlanPrefix = `[MODE: Plan only. You are in planning mode. Follow these phases in order:

PHASE 1 — ENVIRONMENT AUDIT:
The context above includes the local environment (OS, installed tools, runtimes). Review what is available. If the task requires tools/runtimes not installed, note them.

PHASE 2 — CODEBASE SCAN:
Use read_file, glob, and grep to understand the existing codebase structure, patterns, conventions, config files, and dependencies.

PHASE 3 — RESEARCH:
Identify topics you need more context on — frameworks, APIs, libraries, best practices. Use web_search and web_fetch to research them.

PHASE 4 — CLARIFY:
If you have questions for the user, you MUST use this EXACT format with the QUESTIONS: marker on its own line. Do NOT embed questions in the plan text.
QUESTIONS:
1. Single-select question? [Option A / Option B / Option C]
2. Multi-select question? {Option A / Option B / Option C / Option D}
3. Open-ended question?

If you have questions, output ONLY the QUESTIONS: block and STOP — do NOT include PLAN_READY in the same response. Wait for answers before presenting the plan.

PHASE 5 — PLAN:
Only after questions are answered (or if you have none), present a detailed plan with prerequisites, step-by-step changes with file paths, new files vs existing files to modify, dependencies to install, commands to run, and how to verify it works.

RULES:
- Do NOT make changes (no write_file, edit_file).
- Do NOT run destructive or modifying commands.
- You MAY use: read_file, glob, grep, web_search, web_fetch, exec (read-only commands only like ls, cat, which, --version).
- Do NOT put questions and PLAN_READY in the same response — ask first, then plan after answers.
- End your plan with "PLAN_READY" on its own line.]

`

// PlanExecuteMsg — port of acorn/constants.py:PLAN_EXECUTE_MSG.
const PlanExecuteMsg = `[The user has approved the plan above. Switch to execute mode and implement it now. Proceed step by step, executing all the changes you outlined.]`

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Off-thread messages: permissions layer asking to open a modal.
	if om, ok := msg.(openPermModalMsg); ok {
		m.modal = modalPermission
		m.permission = &permissionModal{
			name:      om.name,
			summary:   om.summary,
			rule:      om.rule,
			dangerous: om.dangerous,
		}
		return m, nil
	}

	// Modal intercept.
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
		m.pushChat("system", fmt.Sprintf("Connected to %s:%d as %s (session %s)",
			m.cfg.Connection.Host, m.cfg.Connection.Port, m.cfg.Connection.User, m.sess))
		_ = m.client.Send(map[string]any{
			"type": "chat:history-request", "sessionId": m.sess, "userName": m.cfg.Connection.User,
		})
		return m, nil

	case connErrorMsg:
		m.connected = false
		m.connErr = msg.err
		m.pushChat("system", "Connection error: "+msg.err)
		m.status = "disconnected"
		return m, nil

	case connClosedMsg:
		m.connected = false
		m.status = "disconnected"
		m.pushChat("system", "Disconnected.")
		return m, nil

	case wsFrameMsg:
		cmd := m.handleFrame(msg.frame)
		return m, tea.Batch(cmd, m.recvCmd())

	case toolHandledMsg:
		return m, m.toolCmd()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

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
		if !m.contextSent {
			content = GatherContext(m.cwd) + "\n\n" + content
			m.contextSent = true
		}
		if m.planMode {
			content = PlanPrefix + content
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

func (m *Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	m.input.Reset()
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return m, nil
	}
	cmd := parts[0]

	switch cmd {
	case "/help":
		m.pushChat("system", SlashHelp())
	case "/clear":
		m.messages = m.messages[:0]
		m.rerenderViewport()
		_ = m.client.Send(map[string]any{"type": "chat:clear", "sessionId": m.sess})
	case "/new":
		m.sess = ComputeSessionID(m.cfg.Connection.User, m.cwd)
		m.messages = m.messages[:0]
		m.contextSent = false
		m.rerenderViewport()
		m.pushChat("system", "New session: "+m.sess)
	case "/resume":
		if len(parts) < 2 {
			m.pushChat("system", "Usage: /resume <sessionId>")
			return m, nil
		}
		m.sess = parts[1]
		m.messages = m.messages[:0]
		m.rerenderViewport()
		_ = m.client.Send(map[string]any{"type": "chat:history-request", "sessionId": m.sess, "userName": m.cfg.Connection.User})
		m.pushChat("system", "Resumed: "+m.sess)
	case "/quit":
		m.client.Close()
		return m, tea.Quit
	case "/stop":
		m.exec.AbortCurrent()
		_ = m.client.Send(map[string]any{"type": "chat:stop", "sessionId": m.sess})
		m.pushChat("system", "Stop requested.")
	case "/plan":
		m.planMode = !m.planMode
		label := "execute"
		if m.planMode {
			label = "plan"
		}
		m.pushChat("system", "Mode → "+label)
	case "/status":
		m.pushChat("system", fmt.Sprintf("server=%s:%d user=%s session=%s planMode=%t mode=%s",
			m.cfg.Connection.Host, m.cfg.Connection.Port, m.cfg.Connection.User, m.sess, m.planMode, m.perms.Mode()))
	case "/theme":
		if len(parts) >= 2 {
			m.theme = themeForName(parts[1])
			m.pushChat("system", "Theme → "+m.theme.Name)
			m.rerenderViewport()
		} else {
			m.pushChat("system", "Themes: "+strings.Join(ThemeNames(), ", "))
		}
	case "/mode":
		if len(parts) < 2 {
			m.pushChat("system", "Usage: /mode <auto|ask|locked|yolo|rules>")
			return m, nil
		}
		switch parts[1] {
		case "auto":
			m.perms.SetMode(PermAuto)
			m.pushChat("system", "Perms → auto (non-dangerous auto-approved)")
		case "ask":
			m.perms.SetMode(PermAsk)
			m.pushChat("system", "Perms → ask (prompt per call)")
		case "locked":
			m.perms.SetMode(PermLocked)
			m.pushChat("system", "Perms → locked (deny all writes/exec)")
		case "yolo":
			m.perms.SetMode(PermYolo)
			m.pushChat("system", "Perms → yolo (approve everything)")
		case "rules":
			rs := m.perms.Rules()
			if len(rs) == 0 {
				m.pushChat("system", "No session allow rules")
			} else {
				m.pushChat("system", "Allow rules:\n"+strings.Join(rs, "\n"))
			}
		default:
			m.pushChat("system", "Unknown mode: "+parts[1])
		}
	case "/approve-all":
		m.perms.SetMode(PermAuto)
		m.pushChat("system", "Perms → auto")
	case "/approve-all-dangerous":
		m.perms.SetMode(PermYolo)
		m.pushChat("system", "Perms → yolo")
	default:
		m.pushChat("system", "Unknown command: "+cmd+"  (type /help)")
	}
	return m, nil
}

func (m *Model) handleFrame(f conn.Frame) tea.Cmd {
	switch f.Type {
	case "chat:start":
		m.startStream()
	case "chat:delta":
		var v proto.ChatDelta
		_ = json.Unmarshal(f.Raw, &v)
		m.appendDelta(v.Text)
	case "chat:done":
		m.endStream()
		m.generating = false
		m.status = ""
		m.postStreamChecks()
	case "chat:thinking":
		m.status = "thinking…"
	case "chat:status":
		var v proto.ChatStatus
		_ = json.Unmarshal(f.Raw, &v)
		m.handleStatus(v)
	case "chat:tool":
		var v proto.ChatTool
		_ = json.Unmarshal(f.Raw, &v)
		n := v.Name
		if n == "" {
			n = v.Tool
		}
		if n != "" {
			m.status = "⚙ " + n
		}
	case "chat:history":
		var v proto.ChatHistory
		_ = json.Unmarshal(f.Raw, &v)
		for _, h := range v.Messages {
			role := h.Role
			if role != "user" && role != "assistant" {
				role = "system"
			}
			m.messages = append(m.messages, chatMsg{Role: role, Text: h.Text})
		}
		m.rerenderViewport()
	case "chat:error":
		var v proto.ChatError
		_ = json.Unmarshal(f.Raw, &v)
		m.pushChat("system", "chat error: "+v.Error)
		m.generating = false
		m.status = ""
	case "chat:busy":
		m.pushChat("system", "Server: session busy (another client may be running it)")
	case "code:view":
		var v proto.CodeView
		_ = json.Unmarshal(f.Raw, &v)
		label := "read"
		if v.IsNew {
			label = "new"
		}
		m.pushChat("system", fmt.Sprintf("📄 %s %s", label, v.Path))
	case "code:diff":
		var v proto.CodeDiff
		_ = json.Unmarshal(f.Raw, &v)
		m.pushChat("system", fmt.Sprintf("✏️  edit %s", v.Path))
	case "ask_user":
		var v proto.AskUser
		_ = json.Unmarshal(f.Raw, &v)
		m.openStructuredQuestion(v)
	case "plan_proposal":
		var v proto.PlanProposal
		_ = json.Unmarshal(f.Raw, &v)
		m.pushChat("system", fmt.Sprintf("[plan] queued #%d: %s — %s", v.ProposalID, v.Tool, v.Summary))
	case "plan_applied":
		var v proto.PlanApplied
		_ = json.Unmarshal(f.Raw, &v)
		for _, r := range v.Results {
			mark := "✓"
			if !r.OK {
				mark = "✗"
			}
			m.pushChat("system", fmt.Sprintf("[plan] %s %s %s", mark, r.Tool, r.Summary))
		}
	case "plan_rejected":
		m.pushChat("system", "[plan] proposals rejected")
	case "plan_mode":
		var v proto.PlanMode
		_ = json.Unmarshal(f.Raw, &v)
		m.planMode = v.Enabled
		label := "execute"
		if m.planMode {
			label = "plan"
		}
		m.pushChat("system", "Mode → "+label+" (remote)")
	case "plan:decision", "plan:decided", "plan:set-mode",
		"plan:show-approval", "interactive:resolved",
		"delegate:config", "tool:awaiting-approval",
		"state:questions":
		// observer relays — already surfaced elsewhere for mobile peers.
	case "conn:error":
		// already surfaced via connErrorMsg path
	}
	return nil
}

func (m *Model) handleStatus(v proto.ChatStatus) {
	switch v.Status {
	case "thinking_start":
		m.status = "thinking…"
	case "thinking_done":
		m.status = ""
	case "tool_exec_start":
		m.status = fmt.Sprintf("⚙ %s %s", v.Tool, v.Detail)
	case "tool_exec_done":
		m.status = fmt.Sprintf("✓ %s · %dms", v.Tool, v.DurationMs)
	case "interjected", "interjection":
		m.status = "interjecting…"
	case "waiting":
		m.status = "waiting…"
	case "truncated":
		m.pushChat("system", "[agent] response hit max_tokens — retrying with smaller output")
	}
}

// postStreamChecks runs after chat:done to detect QUESTIONS: / PLAN_READY.
func (m *Model) postStreamChecks() {
	if m.currentStream != nil || len(m.messages) == 0 {
		return
	}
	last := m.messages[len(m.messages)-1]
	if last.Role != "assistant" {
		return
	}
	hasPlan := m.planMode && strings.Contains(last.Text, "PLAN_READY")
	if hasPlan {
		m.stashedPlan = last.Text
	}
	if qs := parseQuestionsBlock(last.Text); qs != nil {
		m.openQuestionModal(qs)
		return
	}
	if hasPlan {
		m.openPlanModal(m.stashedPlan)
		m.stashedPlan = ""
	}
}

// SlashHelp returns the command help block.
func SlashHelp() string {
	return strings.Join([]string{
		"/help — this list",
		"/new — start a fresh session in this cwd",
		"/clear — clear chat history (server-side too)",
		"/resume <sessionId> — resume a specific session",
		"/quit — exit",
		"/stop — stop the current generation",
		"/plan — toggle plan/execute mode (same as Shift+Tab)",
		"/status — connection info",
		"/theme <name> — switch theme (dark, oak, forest, oled, light)",
		"/mode <auto|ask|locked|yolo|rules> — tool approval mode",
		"/approve-all — shortcut for /mode auto",
		"/approve-all-dangerous — shortcut for /mode yolo",
	}, "\n")
}
