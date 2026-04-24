package app

import "testing"

// Standard prose form — single-select, multi-select, open-ended on
// successive lines. The bedrock test: the agent followed the prompt
// exactly, parser must surface 3 questions with the right shapes.
func TestParseQuestionsBlock_proseSingleAndMulti(t *testing.T) {
	in := `QUESTIONS:
1. Framework? [React / Vue / Svelte]
2. Features? {Auth / DB / API}
3. Project name?
`
	qs := parseQuestionsBlock(in)
	if len(qs) != 3 {
		t.Fatalf("expected 3, got %d", len(qs))
	}
	if len(qs[0].Options) != 3 || qs[0].Multi {
		t.Errorf("q0: want single 3 opts, got multi=%v opts=%v", qs[0].Multi, qs[0].Options)
	}
	if !qs[1].Multi {
		t.Errorf("q1: want multi")
	}
	if len(qs[2].Options) != 0 {
		t.Errorf("q2: want open-ended")
	}
}

// JSON-fenced form (the format the plan-mode prompt now teaches as
// preferred). Mixed single / multi / open in a single block.
func TestParseQuestionsBlock_jsonFenced(t *testing.T) {
	in := "let me know:\n\nQUESTIONS:\n```json\n[\n  {\"text\": \"Framework?\", \"type\": \"single\", \"options\": [\"React\", \"Vue\"]},\n  {\"text\": \"Features?\", \"type\": \"multi\", \"options\": [\"Auth\", \"DB\"]},\n  {\"text\": \"Project name?\", \"type\": \"open\"}\n]\n```\n"
	qs := parseQuestionsBlock(in)
	if len(qs) != 3 {
		t.Fatalf("expected 3 questions, got %d: %#v", len(qs), qs)
	}
	if qs[0].Multi {
		t.Errorf("q0 should be single-select")
	}
	if !qs[1].Multi {
		t.Errorf("q1 should be multi-select")
	}
	if len(qs[2].Options) != 0 {
		t.Errorf("q2 should be open-ended")
	}
}

// Real captured turn: agent uses **QUESTIONS:** (markdown bold marker)
// AND puts blank lines between numbered items. The marker tolerance
// + line-by-line continuation walk should still surface all 3.
func TestParseQuestionsBlock_blankLinesBetweenItems(t *testing.T) {
	in := `Good idea. "A website about Claude" branches in a bunch of directions.

**QUESTIONS:**

1. What's the angle? [Compare / Showcase / Tribute]

2. What's the tone? [Playful / Editorial / Technical]

3. Any interactive features? [Live chat / Playground / Static]
`
	qs := parseQuestionsBlock(in)
	if len(qs) != 3 {
		t.Fatalf("expected 3 questions, got %d: %#v", len(qs), qs)
	}
	for i, q := range qs {
		if q.Text == "" {
			t.Errorf("q%d empty text", i)
		}
		if len(q.Options) != 3 {
			t.Errorf("q%d expected 3 opts, got %v", i, q.Options)
		}
	}
}

// Strictness: no QUESTIONS: marker → no picker. Don't synthesize from
// prose, even if it looks pickable. Regression for v0.1.20 — the
// previous detectInlineOptions / splitProseOrOptions fallbacks
// invented wrong pickers for sentences like "Python or Go or staged
// migration?" Documenting that intentional return-nil here.
func TestParseQuestionsBlock_noMarkerReturnsNil(t *testing.T) {
	in := `Two paths: refactor the Python CLI, finish the Go port, or do a staged migration. Which? Pick one and I'll plan accordingly.

**Option A** — Polish Python.
**Option B** — Finish the Go port.
**Option C** — Staged migration.

Which one?
`
	qs := parseQuestionsBlock(in)
	if qs != nil {
		t.Fatalf("no QUESTIONS: marker → expected nil, got %d questions: %#v", len(qs), qs)
	}
}

// Open-ended question with prose "or" enumeration that USED to trip
// splitProseOrOptions. Now: no synthesis — the question stays
// open-ended and the user types the answer. The point is we don't
// chop the question text.
func TestParseQuestionsBlock_openEndedWithOrPhrasingNotSplit(t *testing.T) {
	in := `QUESTIONS:
1. Do you want to refactor the Python CLI or finish the Go port or a staged migration (improve Python first, then port)?
`
	qs := parseQuestionsBlock(in)
	if len(qs) != 1 {
		t.Fatalf("expected 1 question, got %d: %#v", len(qs), qs)
	}
	if len(qs[0].Options) != 0 {
		t.Errorf("expected open-ended, got options: %v", qs[0].Options)
	}
	// Whole sentence preserved as the question text.
	want := "Do you want to refactor the Python CLI or finish the Go port or a staged migration (improve Python first, then port)?"
	if qs[0].Text != want {
		t.Errorf("text mangled:\n  got:  %q\n  want: %q", qs[0].Text, want)
	}
}
