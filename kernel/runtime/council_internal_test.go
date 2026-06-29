// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// councilFakeProvider varies its reply by the request's Model and whether it is
// the chair's synthesis call (System mentions "chair"), so a council test can
// assert each member spoke and the chair synthesized.
type councilFakeProvider struct{}

func (councilFakeProvider) Name() string { return "council-fake" }
func (councilFakeProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	var text string
	if strings.Contains(strings.ToLower(req.System), "chair") {
		text = "CONSENSUS: the council agrees to ship it.\nDISSENT: none"
	} else {
		// Echo the model so we can prove per-member routing happened.
		text = req.Model + " has spoken."
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: text},
		StopReason: agent.StopEndTurn,
	}, nil
}

func openCouncilKernel(t *testing.T, members []CouncilMember) *Kernel {
	t.Helper()
	k, err := Open(Config{
		BaseDir:        t.TempDir(),
		Provider:       councilFakeProvider{},
		Tools:          map[string]agent.Tool{},
		CouncilMembers: func() []CouncilMember { return members },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func TestCouncil_DeliberatesAndSynthesizes(t *testing.T) {
	members := []CouncilMember{{Seat: "Alpha", Model: "model-a"}, {Seat: "Beta", Model: "model-b"}}
	k := openCouncilKernel(t, members)

	res, err := k.Council(context.Background(), "corr-council", "ship or wait?", nil, 1)
	if err != nil {
		t.Fatalf("Council: %v", err)
	}
	if res.Consensus != "the council agrees to ship it." {
		t.Errorf("consensus = %q", res.Consensus)
	}
	if res.Dissent != "" {
		t.Errorf("dissent should be dropped (none), got %q", res.Dissent)
	}
	// Opening round (2) + 1 deliberation round (2) = 4 opinions, both models present.
	if len(res.Opinions) != 4 {
		t.Fatalf("opinions = %d, want 4", len(res.Opinions))
	}
	models := map[string]bool{}
	for _, op := range res.Opinions {
		models[op.Model] = true
		if op.Text == "" {
			t.Errorf("empty opinion: %+v", op)
		}
	}
	if !models["model-a"] || !models["model-b"] {
		t.Errorf("both members should have spoken: %v", models)
	}

	// Events journaled for the audit trail.
	var convened, opinions, consensus int
	_ = k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindCouncilConvened:
			convened++
		case event.KindCouncilOpinion:
			opinions++
		case event.KindCouncilConsensus:
			consensus++
		}
		return nil
	})
	if convened != 1 || consensus != 1 || opinions != 4 {
		t.Errorf("events: convened=%d opinions=%d consensus=%d", convened, opinions, consensus)
	}
}

func TestCouncil_UsesDefaultMembershipAndRejectsEmpty(t *testing.T) {
	// Default membership supplied by cfg.CouncilMembers (members arg nil).
	k := openCouncilKernel(t, []CouncilMember{{Seat: "Solo", Model: "model-x"}})
	res, err := k.Council(context.Background(), "c1", "q", nil, 0)
	if err != nil {
		t.Fatalf("Council with default membership: %v", err)
	}
	if len(res.Members) != 1 || res.Members[0].Model != "model-x" {
		t.Errorf("default membership not used: %+v", res.Members)
	}

	// No members anywhere → a clear error, not a panic.
	empty := openCouncilKernel(t, nil)
	if _, err := empty.Council(context.Background(), "c2", "q", nil, 1); err == nil {
		t.Error("empty council should error")
	}
	// Empty question → error.
	if _, err := k.Council(context.Background(), "c3", "   ", nil, 1); err == nil {
		t.Error("empty question should error")
	}
}

// councilCapturingProvider records every user prompt it's given (concurrency-safe)
// so a test can prove the grounding (date + brief) reached each member and the
// chair. It also answers as the chair when its System mentions "chair".
type councilCapturingProvider struct {
	mu      sync.Mutex
	prompts []string
}

func (p *councilCapturingProvider) Name() string { return "council-capture" }
func (p *councilCapturingProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.mu.Lock()
	for _, m := range req.Messages {
		if m.Role == agent.RoleUser {
			p.prompts = append(p.prompts, m.Content)
		}
	}
	p.mu.Unlock()
	text := "noted."
	if strings.Contains(strings.ToLower(req.System), "chair") {
		text = "CONSENSUS: ok.\nDISSENT: none"
	}
	return &agent.CompletionResponse{Message: agent.Message{Role: agent.RoleAssistant, Content: text}, StopReason: agent.StopEndTurn}, nil
}

// allPromptsContain reports whether every captured user prompt includes sub (and
// at least one was captured).
func (p *councilCapturingProvider) allPromptsContain(sub string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pr := range p.prompts {
		if !strings.Contains(pr, sub) {
			return false
		}
	}
	return len(p.prompts) > 0
}

// fakeSearchTool stands in for the web_search tool, returning a fixed result page
// in the websearch wire shape and counting its calls.
type fakeSearchTool struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeSearchTool) Definition() agent.ToolDef { return agent.ToolDef{Name: "web_search"} }
func (f *fakeSearchTool) Invoke(_ context.Context, _ json.RawMessage) (agent.Result, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return agent.Result{Output: `{"query":"q","count":1,"results":[{"title":"Mars news","url":"https://example.com/mars","snippet":"A rover landed."}]}`}, nil
}
func (f *fakeSearchTool) callCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

func openGroundedCouncil(t *testing.T, prov agent.Provider, members []CouncilMember, tools map[string]agent.Tool, ws bool) *Kernel {
	t.Helper()
	k, err := Open(Config{
		BaseDir:          t.TempDir(),
		Provider:         prov,
		Tools:            tools,
		CouncilMembers:   func() []CouncilMember { return members },
		CouncilWebSearch: ws,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

func TestCouncil_GroundsWithDateAndBrief(t *testing.T) {
	prov := &councilCapturingProvider{}
	tool := &fakeSearchTool{}
	members := []CouncilMember{{Seat: "Alpha", Model: "model-a"}, {Seat: "Beta", Model: "model-b"}}
	k := openGroundedCouncil(t, prov, members, map[string]agent.Tool{"web_search": tool}, true)

	res, err := k.Council(context.Background(), "corr-ground", "is there water on mars?", nil, 1)
	if err != nil {
		t.Fatalf("Council: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if res.AsOf != today {
		t.Errorf("AsOf = %q, want %q", res.AsOf, today)
	}
	if !strings.Contains(res.Brief, "Mars news") || !strings.Contains(res.Brief, "rover landed") {
		t.Errorf("brief missing search hit: %q", res.Brief)
	}
	// One shared search for the whole panel, not one per member/round.
	if c := tool.callCount(); c != 1 {
		t.Errorf("web_search called %d times, want 1 (shared brief)", c)
	}
	// Every member + chair prompt carried the date and the brief.
	if !prov.allPromptsContain(today) {
		t.Error("not all prompts carried today's date")
	}
	if !prov.allPromptsContain("Mars news") {
		t.Error("not all prompts carried the research brief")
	}
	// council.brief event published exactly once for the live UI.
	var briefs int
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindCouncilBrief {
			briefs++
		}
		return nil
	})
	if briefs != 1 {
		t.Errorf("council.brief events = %d, want 1", briefs)
	}
}

func TestCouncil_DateOnlyWhenSearchDisabled(t *testing.T) {
	prov := &councilCapturingProvider{}
	tool := &fakeSearchTool{}
	k := openGroundedCouncil(t, prov, []CouncilMember{{Seat: "Solo", Model: "model-x"}}, map[string]agent.Tool{"web_search": tool}, false)

	res, err := k.Council(context.Background(), "c-off", "q", nil, 0)
	if err != nil {
		t.Fatalf("Council: %v", err)
	}
	if res.Brief != "" {
		t.Errorf("brief should be empty when web search off: %q", res.Brief)
	}
	if c := tool.callCount(); c != 0 {
		t.Errorf("web_search must not be called when off, got %d", c)
	}
	today := time.Now().Format("2006-01-02")
	if res.AsOf != today {
		t.Errorf("date should still be set: %q", res.AsOf)
	}
	if !prov.allPromptsContain(today) {
		t.Error("date should still reach every prompt when search off")
	}
}

func TestSplitConsensusDissent(t *testing.T) {
	cases := []struct{ in, wantC, wantD string }{
		{"CONSENSUS: do X.\nDISSENT: Beta prefers Y.", "do X.", "Beta prefers Y."},
		{"CONSENSUS: do X.\nDISSENT: none", "do X.", ""},
		{"just a plain answer", "just a plain answer", ""},
		// Markdown-header style (what real chair models actually emit).
		{"## CONSENSUS\n\nUse tabs.\n\n## DISSENT\n\nGamma prefers spaces.", "Use tabs.", "Gamma prefers spaces."},
		{"## Consensus\nShip it.\n## Dissent\nnone", "Ship it.", ""},
		{"**CONSENSUS:** do the thing", "do the thing", ""},
	}
	for _, c := range cases {
		gotC, gotD := splitConsensusDissent(c.in)
		if gotC != c.wantC || gotD != c.wantD {
			t.Errorf("split(%q) = (%q,%q), want (%q,%q)", c.in, gotC, gotD, c.wantC, c.wantD)
		}
	}
}
