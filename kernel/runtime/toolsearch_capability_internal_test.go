// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)

// tool_search is injected per RUN (loopconfig, when AGEZT_TOOL_DISCOVERY_MAX
// defers the catalog), so the capability guards — which walk k.Tools() — never
// saw it. It declared no capability and edict's name switch has no case for
// it, so it resolved to the unknown capability "tool_search", which Edict
// DEFAULT-DENIES: tool discovery was dead exactly when it was switched on.
func TestToolSearch_DeclaresAGovernedCapability(t *testing.T) {
	tools := withToolSearch(map[string]agent.Tool{"echo": echoTool{}})
	ts, ok := tools[toolSearchName]
	if !ok {
		t.Fatal("withToolSearch did not inject tool_search")
	}
	def := ts.Definition()
	in := json.RawMessage(`{"query":"files"}`)
	axis := def.Capability.For(in)
	if !edict.KnownCapability(axis) {
		t.Fatalf("tool_search resolves to %q, which Edict does not govern — every call is default-denied", axis)
	}
	out := edict.New(edict.Options{}).Decide(edict.Capability(axis), string(in))
	if out.Decision != edict.DecisionAllow {
		t.Fatalf("tool_search decides %v (%s) under the default posture; discovery must be callable", out.Decision, out.Reason)
	}
}

type echoTool struct{}

func (echoTool) Definition() agent.ToolDef { return agent.ToolDef{Name: "echo", Description: "echo"} }
func (echoTool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: string(raw)}, nil
}
