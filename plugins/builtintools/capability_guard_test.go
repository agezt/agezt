// SPDX-License-Identifier: MIT

package builtintools

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/edict"
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
