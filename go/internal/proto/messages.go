// Package proto defines the WebSocket message shapes shared with the SPORE
// web gateway. Names mirror acorn/protocol.py and gateways/web.js.
//
// For inbound frames the app layer holds onto the raw JSON (via
// conn.Frame.Raw) and decodes specific subtypes on demand. We still keep
// typed helpers here so the hot path (chat streaming) doesn't churn alloc.
package proto

import (
	"encoding/json"
	"time"
)

// ChatStart — no fields we care about.
type ChatStart struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId,omitempty"`
}

// ChatDelta — streaming text chunk.
type ChatDelta struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	SessionID string `json:"sessionId,omitempty"`
}

// ChatThinking — <think> token chunk.
type ChatThinking struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ChatStatus — heartbeat / progress indicator during a turn.
// status ∈ {thinking_start, thinking, thinking_done,
//          tool_exec_start, tool_exec_done, interjection, interjected, waiting}
type ChatStatus struct {
	Type        string `json:"type"`
	Status      string `json:"status"`
	Tool        string `json:"tool,omitempty"`
	Detail      string `json:"detail,omitempty"`
	ResultChars int    `json:"resultChars,omitempty"`
	DurationMs  int    `json:"durationMs,omitempty"`
	Iteration   int    `json:"iteration,omitempty"`
	Tokens      int    `json:"tokens,omitempty"`
	Count       int    `json:"count,omitempty"`
}

// ChatTool — a tool call happened; UI may highlight.
type ChatTool struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Tool string `json:"tool,omitempty"`
}

// ChatDone — end of turn with usage.
type ChatDone struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	Usage      *Usage         `json:"usage,omitempty"`
	Iterations int            `json:"iterations,omitempty"`
	ToolUsage  map[string]int `json:"toolUsage,omitempty"`
}

type Usage struct {
	InputTokens            int `json:"input_tokens,omitempty"`
	OutputTokens           int `json:"output_tokens,omitempty"`
	CacheReadInputTokens   int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// ChatError — fatal error mid-turn.
type ChatError struct {
	Type  string `json:"type"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

// ChatHistory — replayed when joining a session.
type ChatHistory struct {
	Type      string           `json:"type"`
	SessionID string           `json:"sessionId,omitempty"`
	Messages  []HistoryMessage `json:"messages,omitempty"`
}

type HistoryMessage struct {
	Role string    `json:"role"`
	Text string    `json:"text"`
	TS   time.Time `json:"-"`
}

// ChatBusy — server tells us the session is currently mid-turn (e.g. another tab started one).
type ChatBusy struct{ Type string `json:"type"` }

// ChatStopped / ChatCleared — /stop and /clear roundtrips.
type ChatStopped struct{ Type string `json:"type"` }
type ChatCleared struct{ Type string `json:"type"` }

// ToolRequest — server → CLI: "please execute this tool locally and send
// tool:result back." Matches acorn/tools/executor.py inputs.
type ToolRequest struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolAck — synchronous acknowledgment so server knows we're alive.
type ToolAck struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ToolResult — CLI → server with the executed result.
type ToolResult struct {
	Type   string      `json:"type"`
	ID     string      `json:"id"`
	Result any         `json:"result"`
}

// CodeView / CodeDiff — optional code viewer events streamed alongside
// read_file / edit_file tool runs.
type CodeView struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Language string `json:"language,omitempty"`
	IsNew    bool   `json:"isNew,omitempty"`
}

type CodeDiff struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

// AskUser — new SPORE structured question tool.
type AskUser struct {
	Type     string   `json:"type"`
	QID      string   `json:"qid"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
}

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// AskUserAnswer — CLI → server.
type AskUserAnswer struct {
	Type   string `json:"type"`
	QID    string `json:"qid"`
	Answer string `json:"answer"`
}

// Plan-mode (web queue variant). Acorn uses its own prose-based plan flow
// so these just surface as system messages when seen.
type PlanProposal struct {
	Type       string `json:"type"`
	ProposalID int    `json:"proposalId"`
	Sequence   int    `json:"sequence"`
	Tool       string `json:"tool"`
	Summary    string `json:"summary"`
}

type PlanApplied struct {
	Type    string           `json:"type"`
	Results []ProposalResult `json:"results"`
}

type ProposalResult struct {
	ProposalID int    `json:"proposalId"`
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Summary    string `json:"summary,omitempty"`
	Error      string `json:"error,omitempty"`
}

type PlanMode struct {
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

// ── Companion app observer protocol ───────────────────────────────────

// PlanShowApproval — CLI → peers: the agent produced a plan, observers
// should render an approval UI.
type PlanShowApproval struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// PlanDecision — companion → CLI: execute / revise / cancel.
type PlanDecision struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	Feedback string `json:"feedback,omitempty"`
}

// StateQuestions — CLI → peers: we're collecting questions, mobile should
// show the same sheet.
type StateQuestions struct {
	Type      string         `json:"type"`
	Questions []QuestionItem `json:"questions"`
}

type QuestionItem struct {
	Text    string   `json:"text"`
	Options []string `json:"options,omitempty"`
	Multi   bool     `json:"multi,omitempty"`
	Index   int      `json:"index,omitempty"`
}

// InteractiveResolved — CLI → peers: dismiss any active sheet for this kind.
type InteractiveResolved struct {
	Type string `json:"type"`
	Kind string `json:"kind"`
}

// ToolAwaitingApproval — CLI → peers: a dangerous tool is waiting for the
// user to approve. Mobile can render a prompt and reply with ToolApprove.
type ToolAwaitingApproval struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Summary   string `json:"summary"`
	Dangerous bool   `json:"dangerous"`
}

// ToolApprove — companion → CLI: allowed/denied.
type ToolApprove struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Allowed bool   `json:"allowed"`
}
