// SPDX-License-Identifier: MIT

package conductor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
)

type fakeRunner struct {
	gotCfg runtime.ConductorConfig
	res    runtime.ConductorResult
	err    error
}

func (f *fakeRunner) Conduct(_ context.Context, _ string, cfg runtime.ConductorConfig) (runtime.ConductorResult, error) {
	f.gotCfg = cfg
	return f.res, f.err
}

func TestConductor_Invoke(t *testing.T) {
	fr := &fakeRunner{res: runtime.ConductorResult{
		Task:   "add(2,2)",
		Answer: "def add(a,b): return a+b",
		Passed: true,
		Roles:  map[string]string{"thinker": "m1", "worker": "m2", "verifier": "m3"},
		Rounds: 1,
		Steps: []runtime.ConductorStep{
			{Round: 0, Role: "thinker", Model: "m1", Text: "plan"},
			{Round: 1, Role: "worker", Model: "m2", Text: "code"},
			{Round: 1, Role: "verifier", Model: "m3", Verdict: "pass", Reason: "ran cleanly"},
		},
	}}
	tool := New()
	tool.SetRunner(fr)

	r, err := tool.Invoke(context.Background(), json.RawMessage(`{"task":"add(2,2)","worker":"@fast","max_rounds":3,"plan":true}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.Output)
	}
	if fr.gotCfg.Task != "add(2,2)" || fr.gotCfg.Worker != "@fast" || fr.gotCfg.MaxRounds != 3 || !fr.gotCfg.Plan {
		t.Errorf("runner got cfg=%+v", fr.gotCfg)
	}
	for _, want := range []string{`"passed": true`, `"answer"`, `"steps"`, "verifier"} {
		if !strings.Contains(r.Output, want) {
			t.Errorf("output missing %q:\n%s", want, r.Output)
		}
	}
}

func TestConductor_RequiresTask(t *testing.T) {
	tool := New()
	tool.SetRunner(&fakeRunner{})
	r, err := tool.Invoke(context.Background(), json.RawMessage(`{"task":"   "}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError || !strings.Contains(r.Output, "task required") {
		t.Errorf("want task-required error, got %q (isErr=%v)", r.Output, r.IsError)
	}
}

func TestConductor_Unavailable(t *testing.T) {
	tool := New() // no runner
	r, err := tool.Invoke(context.Background(), json.RawMessage(`{"task":"x"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError || !strings.Contains(r.Output, "unavailable") {
		t.Errorf("want unavailable error, got %q (isErr=%v)", r.Output, r.IsError)
	}
}

func TestConductor_Definition(t *testing.T) {
	d := New().Definition()
	if d.Name != "conductor" {
		t.Fatalf("name = %q", d.Name)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(d.InputSchema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "task" {
		t.Errorf("required = %v, want [task]", schema.Required)
	}
}
