// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"strings"
	"testing"

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
