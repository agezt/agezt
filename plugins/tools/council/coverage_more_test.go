// SPDX-License-Identifier: MIT

package council

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
)

type covRunner struct {
	gotCorr    string
	gotQ       string
	gotMembers []runtime.CouncilMember
	gotRounds  int
	res        runtime.CouncilResult
	err        error
}

func (f *covRunner) Council(_ context.Context, corr, q string, members []runtime.CouncilMember, rounds int) (runtime.CouncilResult, error) {
	f.gotCorr = corr
	f.gotQ = q
	f.gotMembers = members
	f.gotRounds = rounds
	return f.res, f.err
}

func TestCouncilCoverageDefinitionAndHelpers(t *testing.T) {
	tool := New()
	if tool.runner != nil {
		t.Fatal("New should leave runner nil")
	}
	tool.SetRunner(&covRunner{})
	if tool.runner == nil {
		t.Fatal("SetRunner should wire runner")
	}

	def := tool.Definition()
	if def.Name != "council" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectReversible {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectReversible)
	}
	schema := string(def.InputSchema)
	for _, want := range []string{`"question"`, `"rounds"`, `"required"`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema should include %q, got %s", want, schema)
		}
	}
}

func TestCouncilCoverageInvokeValidation(t *testing.T) {
	// Parse error: hard.
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Unbound runner.
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unavailable") {
		t.Fatalf("unavailable = %+v", res)
	}

	// Empty question.
	tool := New()
	tool.SetRunner(&covRunner{})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"question":""}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "question required") {
		t.Fatalf("empty question = %+v", res)
	}

	// Runner error.
	tool.SetRunner(&covRunner{err: errors.New("boom")})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"question":"q"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "boom") {
		t.Fatalf("runner error = %+v", res)
	}
}

func TestCouncilCoverageInvokeHappyPath(t *testing.T) {
	r := &covRunner{res: runtime.CouncilResult{
		Consensus: "yes",
		Dissent:   "no",
		Members:   []runtime.CouncilMember{{Seat: "s1", Model: "m1"}, {Seat: "s2", Model: "m2"}},
		Rounds:    2,
		AsOf:      "2026-07-07",
		Brief:     "summary",
		Opinions: []runtime.Opinion{
			{Seat: "s1", Model: "m1", Round: 1, Text: "ok"},
			{Seat: "s2", Model: "m2", Round: 2, Text: "", Error: "timeout"},
		},
	}}
	tool := New()
	tool.SetRunner(r)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"question":"go?","rounds":3}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	if r.gotQ != "go?" || r.gotRounds != 3 {
		t.Fatalf("runner got %+v", r)
	}
	for _, want := range []string{`"consensus": "yes"`, `"dissent": "no"`, `"members"`, `"rounds": 2`, `"opinions"`, `"as_of"`, `"brief"`} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q: %s", want, res.Output)
		}
	}
	// Opinion with error surfaces an "error" key.
	if !strings.Contains(res.Output, `"error": "timeout"`) {
		t.Fatalf("opinion error missing: %s", res.Output)
	}
}
