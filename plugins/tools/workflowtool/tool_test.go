// SPDX-License-Identifier: MIT

package workflowtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workflowapi"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
)

// fakeKernel records calls and serves a real (temp-dir) workflow store so
// list/show exercise the same code paths the daemon does.
//
// The rows it speaks are the contract's: the plugin may not name the module
// in its own code, and the store is only here because the fake needs a real
// implementation to answer with.
type fakeKernel struct {
	store      *workflow.Store
	savedCorr  string
	ranRef     string
	ranPayload any
	enabledRef string
	enabledVal bool
}

func newFakeKernel(t *testing.T) *fakeKernel {
	t.Helper()
	st, err := workflow.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return &fakeKernel{store: st}
}

func (f *fakeKernel) SaveWorkflow(corr string, w workflowapi.Workflow) (workflowapi.Workflow, bool, error) {
	f.savedCorr = corr
	return f.store.Save(w)
}

func (f *fakeKernel) SetWorkflowEnabled(_ string, ref string, enabled bool) (workflowapi.Workflow, error) {
	f.enabledRef, f.enabledVal = ref, enabled
	return f.store.SetEnabled(ref, enabled)
}

func (f *fakeKernel) RunWorkflow(_ context.Context, _ string, ref string, payload any) (workflowapi.RunResult, error) {
	f.ranRef, f.ranPayload = ref, payload
	return workflowapi.RunResult{
		Executed: []string{"start", "greet"},
		Outputs:  map[string]any{"greet": "hello"},
	}, nil
}

func invoke(t *testing.T, tool *Tool, input string) string {
	t.Helper()
	res, err := tool.Invoke(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("Invoke(%s): %v", input, err)
	}
	return res.Output
}

const graphJSON = `{"name":"greeter","description":"says hi","nodes":[{"id":"start","type":"trigger"},{"id":"greet","type":"transform","config":{"template":"hello {{trigger.payload.name}}"}}],"edges":[{"from":"start","to":"greet"}]}`

func TestWorkflowToolDefinitionStatesReusableChainNotIdentity(t *testing.T) {
	d := New().Definition()
	if !strings.Contains(d.Description, "reusable chains, not agent identities") {
		t.Fatalf("description should separate workflows from agent identities, got %q", d.Description)
	}
	if !strings.Contains(d.Description, "users, agents, schedules, and webhooks can run") {
		t.Fatalf("description should state workflow run surfaces, got %q", d.Description)
	}
}

func TestWorkflowTool_SaveRunEnableLifecycle(t *testing.T) {
	fk := newFakeKernel(t)
	tool := New()
	tool.Bind(fk, fk.store)

	// save: validated by the store, arrives disabled.
	out := invoke(t, tool, `{"op":"save","workflow":`+graphJSON+`}`)
	if !strings.Contains(out, `"name": "greeter"`) || !strings.Contains(out, "created (disabled)") {
		t.Fatalf("save output:\n%s", out)
	}
	if got, _ := fk.store.Get("greeter"); got.Enabled {
		t.Fatal("a fresh save must arrive disabled")
	}

	// run: ref + payload flow through; per-node outputs come back.
	out = invoke(t, tool, `{"op":"run","ref":"greeter","payload":{"name":"ersin"}}`)
	if fk.ranRef != "greeter" {
		t.Fatalf("ranRef = %q", fk.ranRef)
	}
	if p, _ := fk.ranPayload.(map[string]any); p["name"] != "ersin" {
		t.Fatalf("payload = %v", fk.ranPayload)
	}
	if !strings.Contains(out, `"greet": "hello"`) {
		t.Fatalf("run output:\n%s", out)
	}

	// enable arms triggers.
	out = invoke(t, tool, `{"op":"enable","ref":"greeter","enabled":true}`)
	if !strings.Contains(out, `"enabled": true`) || fk.enabledRef != "greeter" || !fk.enabledVal {
		t.Fatalf("enable output:\n%s", out)
	}

	// list + show read the same store.
	out = invoke(t, tool, `{"op":"list"}`)
	if !strings.Contains(out, `"count": 1`) {
		t.Fatalf("list output:\n%s", out)
	}
	out = invoke(t, tool, `{"op":"show","ref":"greeter"}`)
	if !strings.Contains(out, "{{trigger.payload.name}}") {
		t.Fatalf("show must carry the full graph:\n%s", out)
	}
}

func TestWorkflowTool_RefusalsAndValidation(t *testing.T) {
	tool := New()

	// Unbound: friendly error, not a panic.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil || !res.IsError {
		t.Fatalf("unbound: res=%+v err=%v", res, err)
	}

	fk := newFakeKernel(t)
	tool.Bind(fk, fk.store)

	for _, tc := range []struct{ input, want string }{
		{`{"op":"save"}`, `"workflow" object`},
		{`{"op":"save","workflow":{"name":"BAD NAME","nodes":[{"id":"s","type":"trigger"}]}}`, "name must match"},
		{`{"op":"run"}`, `"ref"`},
		{`{"op":"enable","ref":"x"}`, `"enabled"`},
		{`{"op":"show","ref":"ghost"}`, "no workflow"},
		{`{"op":"explode"}`, "unknown op"},
		{`{}`, "op required"},
	} {
		res, err := tool.Invoke(context.Background(), json.RawMessage(tc.input))
		if err != nil {
			t.Fatalf("Invoke(%s): %v", tc.input, err)
		}
		if !res.IsError || !strings.Contains(res.Output, tc.want) {
			t.Fatalf("input %s → %q (want %q)", tc.input, res.Output, tc.want)
		}
	}
}
