// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/reasoningapi"
)

type fakeRunner struct {
	gotQuestion string
	gotRounds   int
	res         reasoningapi.CouncilResult
	err         error
}

func (f *fakeRunner) Council(_ context.Context, _ string, q string, _ []reasoningapi.CouncilMember, rounds int) (reasoningapi.CouncilResult, error) {
	f.gotQuestion = q
	f.gotRounds = rounds
	return f.res, f.err
}

func TestCouncil_Invoke(t *testing.T) {
	fr := &fakeRunner{res: reasoningapi.CouncilResult{
		Consensus: "ship it",
		Dissent:   "Beta worried about tests",
		Members:   []reasoningapi.CouncilMember{{Seat: "Alpha", Model: "m1"}, {Seat: "Beta", Model: "m2"}},
		Rounds:    1,
		Opinions: []reasoningapi.Opinion{
			{Seat: "Alpha", Model: "m1", Round: 0, Text: "yes"},
			{Seat: "Beta", Model: "m2", Round: 0, Text: "maybe"},
		},
	}}
	tool := New()
	tool.SetRunner(fr)

	r, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"ship or wait?","rounds":2}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Output)
	}
	if fr.gotQuestion != "ship or wait?" || fr.gotRounds != 2 {
		t.Errorf("runner got q=%q rounds=%d", fr.gotQuestion, fr.gotRounds)
	}
	if !strings.Contains(r.Output, "ship it") || !strings.Contains(r.Output, "Beta worried") {
		t.Errorf("output missing consensus/dissent: %s", r.Output)
	}
	// Opinions are surfaced.
	if !strings.Contains(r.Output, "Alpha") || !strings.Contains(r.Output, "maybe") {
		t.Errorf("output missing opinions: %s", r.Output)
	}
}

// TestCouncil_SurfacesTheWholeDebate pins the output mapping: the consensus,
// the dissent, the panel that sat, the round count, every recorded opinion
// (with its seat, model, round and text) and the dated brief must reach the
// agent. A mapping that quietly drops a field leaves the agent with a thinner
// debate than the panel produced.
func TestCouncil_SurfacesTheWholeDebate(t *testing.T) {
	fr := &fakeRunner{res: reasoningapi.CouncilResult{
		Consensus: "ship it",
		Dissent:   "Beta worried about tests",
		Members:   []reasoningapi.CouncilMember{{Seat: "Alpha", Model: "m1"}},
		Rounds:    1,
		Opinions: []reasoningapi.Opinion{
			{Seat: "Alpha", Model: "m1", Round: 0, Text: "yes"},
		},
		AsOf:  "2026-10-23",
		Brief: "the brief",
	}}
	tool := New()
	tool.SetRunner(fr)

	r, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"ship or wait?"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Output)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(r.Output), &out); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	for _, key := range []string{"consensus", "dissent", "members", "rounds", "opinions", "as_of", "brief"} {
		if _, ok := out[key]; !ok {
			t.Errorf("output is missing %q", key)
		}
	}
	if out["as_of"] != "2026-10-23" || out["brief"] != "the brief" {
		t.Errorf("the dated brief must reach the agent: as_of=%v brief=%v", out["as_of"], out["brief"])
	}
	members, _ := out["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("members = %v, want the panel that sat", out["members"])
	}
	member, _ := members[0].(map[string]any)
	if member["seat"] != "Alpha" || member["model"] != "m1" {
		t.Errorf("member = %v, want the seat and its model", member)
	}
	opinions, _ := out["opinions"].([]any)
	if len(opinions) != 1 {
		t.Fatalf("opinions = %v, want the recorded opinion", out["opinions"])
	}
	opinion, _ := opinions[0].(map[string]any)
	if opinion["seat"] != "Alpha" || opinion["model"] != "m1" || opinion["text"] != "yes" {
		t.Errorf("opinion = %v, want the seat, the model and the text", opinion)
	}
}

func TestCouncil_Rejections(t *testing.T) {
	// No runner.
	noRunner := New()
	if r, _ := noRunner.Invoke(context.Background(), json.RawMessage(`{"question":"x"}`)); !r.IsError || !strings.Contains(r.Output, "unavailable") {
		t.Errorf("missing runner should report unavailable: %s", r.Output)
	}
	// Empty question.
	tool := New()
	tool.SetRunner(&fakeRunner{})
	if r, _ := tool.Invoke(context.Background(), json.RawMessage(`{"question":"  "}`)); !r.IsError {
		t.Error("empty question should be rejected")
	}
	// Runner error → soft error result.
	tool.SetRunner(&fakeRunner{err: errors.New("no members")})
	if r, _ := tool.Invoke(context.Background(), json.RawMessage(`{"question":"x"}`)); !r.IsError || !strings.Contains(r.Output, "no members") {
		t.Errorf("runner error should surface: %s", r.Output)
	}
}
