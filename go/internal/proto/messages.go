// Package proto defines the WebSocket message envelope shared with the SPORE
// web gateway. The server side is authoritative; see gateways/web.js in the
// spore server for the canonical shapes. This file keeps Go structs for the
// subset that the CLI cares about.
package proto

import "encoding/json"

// In messages (server → CLI).
type In struct {
	Type    string          `json:"type"`
	Raw     json.RawMessage `json:"-"` // the full message for unknown types
	Text    string          `json:"text,omitempty"`
	Error   string          `json:"error,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Detail  string          `json:"detail,omitempty"`
	Status  string          `json:"status,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`

	// chat:history
	Messages []HistoryEntry `json:"messages,omitempty"`

	// ask_user (our new tool) — structured question card
	QID       string   `json:"qid,omitempty"`
	Question  string   `json:"question,omitempty"`
	Options   []Option `json:"options,omitempty"`

	// plan_* (new tool-queue plan mode)
	ProposalID int             `json:"proposalId,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	Results    []ProposalResult `json:"results,omitempty"`

	// usage on chat:done
	Usage     *Usage `json:"usage,omitempty"`
	Iterations int   `json:"iterations,omitempty"`

	// session routing
	SessionID string `json:"sessionId,omitempty"`
}

type HistoryEntry struct {
	Role string `json:"role"`
	Text string `json:"text"`
	TS   string `json:"ts,omitempty"`
}

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type ProposalResult struct {
	ProposalID int    `json:"proposalId"`
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Summary    string `json:"summary,omitempty"`
	Error      string `json:"error,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// Out messages (CLI → server).
type Out struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId,omitempty"`
	Content   string `json:"content,omitempty"`
	UserName  string `json:"userName,omitempty"`
	CWD       string `json:"cwd,omitempty"`

	// ask_user_answer
	QID    string `json:"qid,omitempty"`
	Answer string `json:"answer,omitempty"`

	// chat:history-request
}
