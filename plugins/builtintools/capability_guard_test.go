// SPDX-License-Identifier: MIT

package builtintools

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/toolreg"
	"github.com/agezt/agezt/kernel/warden"
)

// A tool's policy axis is declared in kernel/edict's name switch, a different
// package from the tool itself. Nothing used to connect the two, and a tool
// whose name never reached that switch resolved to an unknown capability —
// which Edict default-DENIES. So shipping a tool without remembering the edict
// edit did not degrade the tool, it killed it: every call refused with
// "no trust level configured for <tool>", on a daemon whose declared posture is
// allow-everything-unless-turned-off.
//
// That is how `conductor` and `market` shipped unusable. These guards make the
// omission fail here, next to the tool registry, instead of at run time.
//
// Also covered: kernel/runtime has the mirror guard for the tools IT registers
// (memory, world, delegate, market, voice, …), which this package cannot see.

// opProbes lists, per tool, the input values that STEER its capability — the
// tools whose axis depends on what the call asks for. Every value the tool's
// input schema enumerates must appear, because each one resolves separately and
// a missing branch is exactly the `file` op=glob bug: the op was implemented and
// advertised in the schema, but the switch had no case for it, so every glob
// call was refused.
var opProbes = map[string][]map[string]string{
	"file": {
		{"op": "read"}, {"op": "write"}, {"op": "append"}, {"op": "list"},
		{"op": "search"}, {"op": "stat"}, {"op": "delete"}, {"op": "replace"},
		{"op": "glob"},
	},
	"artifacts":     {{"op": "list"}, {"op": "read"}, {"op": "delete"}},
	"config":        {{"op": "schema"}, {"op": "get"}, {"op": "set"}, {"op": "register"}, {"op": "unregister"}},
	"http":          {{"method": "GET"}, {"method": "POST"}},
	"homeassistant": {{"operation": "get_states"}, {"operation": "call_service"}},
	"tool_forge":    {{"op": "draft"}, {"op": "test"}, {"op": "update"}, {"op": "request_promotion"}, {"op": "list"}, {"op": "show"}},
	"mcp":           {{"op": "add"}, {"op": "attach"}, {"op": "detach"}, {"op": "list"}, {"op": "remove"}},
	"workflow":      {{"op": "save"}, {"op": "run"}, {"op": "enable"}, {"op": "list"}, {"op": "show"}},
	"db": {
		{"op": "list_collections"}, {"op": "create_collection"}, {"op": "drop_collection"},
		{"op": "insert"}, {"op": "get"}, {"op": "update"}, {"op": "delete"}, {"op": "query"},
	},
}

// probesFor returns the inputs to resolve a capability against: the steering
// values for a multi-axis tool, or one empty object for a single-axis one.
func probesFor(tool string) []map[string]string {
	if p, ok := opProbes[tool]; ok {
		return p
	}
	return []map[string]string{{}}
}

// Every boot tool must resolve to a capability Edict actually governs — for
// EVERY input its schema allows. An unknown capability is default-denied, so a
// gap here is a tool that cannot be called at all.
func TestBootTools_ResolveToGovernedCapabilities(t *testing.T) {
	RegisterAll()
	for _, tool := range bootSpecNames {
		if tool == "plugins" {
			// Not a callable tool: the spec name for the external plugin HOST.
			// Its tools arrive prefixed and declare their axis via the plugin
			// capability manifest (M900).
			continue
		}
		for _, probe := range probesFor(tool) {
			in, err := json.Marshal(probe)
			if err != nil {
				t.Fatalf("%s: marshal probe: %v", tool, err)
			}
			cap := edict.CapabilityForToolCall(tool, in)
			if !edict.KnownCapability(string(cap)) {
				t.Errorf("tool %q with input %v resolves to capability %q, which Edict does not govern — "+
					"an unknown capability is DEFAULT-DENIED, so every such call is refused. "+
					"Map it in kernel/edict/toolmap.go (join an existing axis), or add the axis to "+
					"AllCapabilities() if it genuinely needs its own.", tool, probe, cap)
			}
		}
	}
}

// Phase 3.4 commit B moves the axis from edict's name switch onto the tool's own
// ToolDef.Capability. This is the parity net for that move: for every real boot
// tool and every input its schema allows, what the tool DECLARES must equal what
// the switch resolved before. A wrong annotation silently re-gates a tool — onto
// a looser axis in the worst case — so the migration is only safe with this held
// green tool by tool.
//
// A tool that has not been annotated yet is skipped (it still falls through to
// the switch, unchanged). TestBootTools_DeclareTheirCapability is what stops
// "not yet annotated" from becoming permanent.
func TestBootTools_DeclaredCapabilityMatchesTheNameSwitch(t *testing.T) {
	for name, tool := range buildRealBootTools(t) {
		declared := tool.Definition().Capability
		if declared.IsZero() {
			continue
		}
		for _, probe := range probesFor(name) {
			in, err := json.Marshal(probe)
			if err != nil {
				t.Fatalf("%s: marshal probe: %v", name, err)
			}
			want := edict.CapabilityForToolCall(name, in)
			if got := declared.For(in); got != string(want) {
				t.Errorf("%s with input %v: declares %q but the name switch resolves %q — "+
					"the declaration must reproduce today's gating exactly, or this tool's policy "+
					"axis changes as a side effect of the refactor", name, probe, got, want)
			}
		}
	}
}

// Every boot tool must declare its axis, so the switch is a fallback for the
// DYNAMIC surfaces only (forge_<name>, mcp_<server>_<tool>) rather than the
// place in-tree tools are classified.
func TestBootTools_DeclareTheirCapability(t *testing.T) {
	for name, tool := range buildRealBootTools(t) {
		if tool.Definition().Capability.IsZero() {
			t.Errorf("tool %q declares no Capability — set ToolDef.Capability in the tool's own "+
				"package so its policy axis lives next to its behaviour", name)
		}
	}
}

// buildRealBootTools builds the actual boot registry so the guards read each
// tool's real ToolDef rather than a hand-maintained copy of it.
func buildRealBootTools(t *testing.T) map[string]agent.Tool {
	t.Helper()
	RegisterAll()
	var stderr bytes.Buffer
	set, err := toolreg.BuildAll(toolreg.BuildDeps{
		BaseDir:       t.TempDir(),
		WorkspaceRoot: t.TempDir(),
		Warden:        warden.New(nil),
		Stderr:        &stderr,
		Get:           func(string) string { return "" },
	})
	if err != nil {
		t.Fatalf("BuildAll: %v; stderr=%s", err, stderr.String())
	}
	return set.Tools()
}

// And every governed capability must be allowed by default — the owner's
// max-autonomy posture (M814). This is what makes the guard above sufficient:
// once a capability is known, it is also usable, so "known" is the whole bar.
func TestGovernedCapabilities_AllowedByDefault(t *testing.T) {
	eng := edict.New(edict.Options{})
	for _, cap := range edict.AllCapabilities() {
		out := eng.Decide(cap, "{}")
		if out.Decision != edict.DecisionAllow {
			t.Errorf("capability %q decides %v by default (%s) — DefaultLevels is the "+
				"allow-everything posture; a new axis must not ship restricted without an owner decision",
				cap, out.Decision, out.Reason)
		}
	}
}
