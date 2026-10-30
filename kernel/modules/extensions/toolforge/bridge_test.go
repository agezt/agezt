// SPDX-License-Identifier: MIT

package toolforge

// The SCRIPT-TOOL BRIDGE's own tests (W3.72).
//
// The bridge answers for the forge's ACTIVE rows, so it belongs with them —
// and its rules are the daemon's rules: a registered tool always wins a name
// collision, and a promoted script is offered under the forge_ prefix, which
// also lets the Edict toolmap route every forged call to the `code.exec`
// capability.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// stubTool is a registered tool: the run's own map.
type stubTool struct{ name string }

func (s stubTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: s.name, Description: "registered"}
}

func (s stubTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "registered"}, nil
}

// activeForge builds a service whose catalogue holds one ACTIVE tool.
func activeForge(t *testing.T, conn toolregapi.ScriptRunner, st toolforgeapi.ScriptTool) *Service {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open the forge store: %v", err)
	}
	saved, err := store.Add(st)
	if err != nil {
		t.Fatalf("register the tool: %v", err)
	}
	if _, err := store.RecordTest(saved.ID, true); err != nil {
		t.Fatalf("record the test: %v", err)
	}
	if _, err := store.Promote(saved.ID); err != nil {
		t.Fatalf("promote the tool: %v", err)
	}
	return NewService(extapi.ForgeDependencies{Store: store, ScriptRunner: conn})
}

// TestTheMergeLeavesTheMapUntouchedWithoutAScript: a run with no promoted
// script gets its own tool map back — the same map, not a copy.
func TestTheMergeLeavesTheMapUntouchedWithoutAScript(t *testing.T) {
	svc := NewService(extapi.ForgeDependencies{})
	tools := map[string]toolapi.Tool{"own": stubTool{"own"}}
	got := svc.MergeTools(tools)
	if len(got) != 1 {
		t.Fatalf("the merge invented tools: %v", got)
	}
	if _, ok := got["own"]; !ok {
		t.Fatalf("the merge dropped the registered tool: %v", got)
	}
}

// TestTheMergeOffersThePromotedScript: a promoted script is offered under
// its forge_ name, and a call forwards to the sandbox.
func TestTheMergeOffersThePromotedScript(t *testing.T) {
	runner := &stubRunner{out: "hi"}
	svc := activeForge(t, runner, toolforgeapi.ScriptTool{
		Name: "greet", Description: "greets a name", Language: "python", Code: "print('hi')",
	})

	got := svc.MergeTools(map[string]toolapi.Tool{})
	tool, ok := got["forge_greet"]
	if !ok {
		t.Fatalf("the promoted script is not offered: %v", got)
	}
	def := tool.Definition()
	if !strings.Contains(def.Description, "greets a name") || !strings.Contains(def.Description, "python") {
		t.Fatalf("the offered tool does not describe the script: %+v", def)
	}
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"name":"ersin"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Output != "hi" || res.IsError {
		t.Fatalf("the call did not forward to the sandbox: %+v", res)
	}
	if runner.runs != 1 || runner.language != "python" || runner.input != `{"name":"ersin"}` {
		t.Fatalf("the sandbox did not run the script: %+v", runner)
	}
}

// TestARegisteredToolWinsTheCollision: the run's own tool is not shadowed by
// a forged one.
func TestARegisteredToolWinsTheCollision(t *testing.T) {
	svc := activeForge(t, &stubRunner{out: "hi"}, toolforgeapi.ScriptTool{
		Name: "greet", Description: "greets a name", Language: "python", Code: "print('hi')",
	})
	own := stubTool{"forge_greet"}
	got := svc.MergeTools(map[string]toolapi.Tool{"forge_greet": own})
	if got["forge_greet"].Definition().Description != "registered" {
		t.Fatalf("the forged tool shadowed the registered one: %+v", got["forge_greet"].Definition())
	}
}

// TestTheForgedNameIsTheForgePrefix: the prefix namespaces scripts away from
// built-in tools and routes every forged call to the `code.exec` capability.
func TestTheForgedNameIsTheForgePrefix(t *testing.T) {
	if got := ForgedToolName("greet"); got != "forge_greet" {
		t.Fatalf("the callable name is wrong: %q", got)
	}
}

// TestAFailedScriptIsAnErrorResult: a failing script is an error RESULT, not
// a crashed run.
func TestAFailedScriptIsAnErrorResult(t *testing.T) {
	runner := &stubRunner{out: "boom", isErr: true}
	svc := activeForge(t, runner, toolforgeapi.ScriptTool{
		Name: "greet", Description: "greets a name", Language: "python", Code: "print('hi')",
	})
	got := svc.MergeTools(map[string]toolapi.Tool{})
	res, err := got["forge_greet"].Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a failing script must not fail the run: %v", err)
	}
	if !res.IsError || res.Output != "boom" {
		t.Fatalf("the failure was not reported as an error result: %+v", res)
	}
}
