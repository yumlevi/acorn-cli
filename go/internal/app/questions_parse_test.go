package app

import "testing"

// Regression for the "blank lines between numbered items truncated
// the question set to 1" bug that shipped in v0.1.11. Real assistant
// output captured from sporebfl's session DB after the user reported
// "didn't show me the question ui in acorn" on v0.1.12.
func TestParseQuestionsBlock_blankLinesBetweenItems(t *testing.T) {
	in := `Good idea. "A website about Claude" branches in a bunch of directions — a comparison tool, a fan/tribute page, a documentation hub, or something more creative.

**QUESTIONS:**

1. **What's the angle?** Compare Claude to other LLMs, showcase what Claude can do with demos, a creative tribute/portrait, or document its history and capabilities?

2. **What's the tone?** Playful and fan-like, clean and editorial (like a product page), or technical and informational?

3. **Any interactive features?** Live chat demo, prompt playground, model comparison table, or static content only?
`
	qs := parseQuestionsBlock(in)
	if len(qs) != 3 {
		t.Fatalf("expected 3 questions, got %d: %#v", len(qs), qs)
	}
	if qs[0].Multi || qs[1].Multi || qs[2].Multi {
		t.Errorf("none of these should be multi-select")
	}
	for i, q := range qs {
		if q.Text == "" {
			t.Errorf("question %d has empty text", i)
		}
	}
}

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
