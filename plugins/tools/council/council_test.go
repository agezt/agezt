// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
)

type fakeRunner struct {
	gotQuestion string
	gotRounds   int
	res         runtime.CouncilResult
	err         error
}

func (f *fakeRunner) Council(_ context.Context, _ string, q string, _ []runtime.CouncilMember, rounds int) (runtime.CouncilResult, error) {
	f.gotQuestion = q
	f.gotRounds = rounds
	return f.res, f.err
}

func TestCouncil_Invoke(t *testing.T) {
	fr := &fakeRunner{res: runtime.CouncilResult{
		Consensus: "ship it",
		Dissent:   "Beta worried about tests",
		Members:   []runtime.CouncilMember{{Seat: "Alpha", Model: "m1"}, {Seat: "Beta", Model: "m2"}},
		Rounds:    1,
		Opinions: []runtime.Opinion{
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
