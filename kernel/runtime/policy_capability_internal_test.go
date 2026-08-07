// SPDX-License-Identifier: MIT

package runtime

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)

// capabilityFor's precedence ladder (Phase 3.4 commit B). Internal because the
// interesting cases are about which SOURCE wins, and constructing a kernel with
// a specific plugin-manifest overlay is a package-private concern.

func shellCall() agent.ToolCall {
	return agent.ToolCall{Name: "shell", Input: json.RawMessage(`{"command":"echo hi"}`)}
}

// A valid declaration on the tool's own def wins over everything else.
func TestCapabilityFor_DeclarationWins(t *testing.T) {
	k := &Kernel{toolCaps: map[string]edict.Capability{"shell": edict.CapMemory}}
	def := agent.ToolDef{Name: "shell", Capability: agent.ToolCapability{Name: string(edict.CapCodeExec)}}
	if got := k.capabilityFor(shellCall(), def); got != edict.CapCodeExec {
		t.Errorf("got %q, want the declared %q (it must beat both the manifest overlay and the name switch)", got, edict.CapCodeExec)
	}
}

// A declaration naming an axis Edict does not govern must be IGNORED, not
// honoured. Honouring it would resolve to an unknown capability — which is
// DEFAULT-DENIED, silently killing the tool. That is precisely the failure this
// field was introduced to end, so a typo must not be able to reintroduce it.
func TestCapabilityFor_UnknownDeclarationIsIgnoredNotHonoured(t *testing.T) {
	k := &Kernel{}
	def := agent.ToolDef{Name: "shell", Capability: agent.ToolCapability{Name: "shel"}}
	if got := k.capabilityFor(shellCall(), def); got != edict.CapShell {
		t.Errorf("got %q, want the name switch's %q — a typo must degrade to the fallback, never to default-deny", got, edict.CapShell)
	}
}

// Same rule for a multi-axis declaration whose matched value is a typo: that
// call falls through, while its well-formed siblings keep working.
func TestCapabilityFor_UnknownValueAxisFallsThroughPerCall(t *testing.T) {
	k := &Kernel{}
	def := agent.ToolDef{
		Name: "file",
		Capability: agent.ToolCapability{
			Field:   "op",
			ByValue: map[string]string{"read": string(edict.CapFileRead), "delete": "file.nuke"},
		},
	}
	if got := k.capabilityFor(agent.ToolCall{Name: "file", Input: json.RawMessage(`{"op":"read"}`)}, def); got != edict.CapFileRead {
		t.Errorf("op=read → %q, want %q", got, edict.CapFileRead)
	}
	if got := k.capabilityFor(agent.ToolCall{Name: "file", Input: json.RawMessage(`{"op":"delete"}`)}, def); got != edict.CapFileDelete {
		t.Errorf("op=delete → %q, want the name switch's %q (the bogus axis must be ignored)", got, edict.CapFileDelete)
	}
}

// With no declaration, the plugin manifest overlay is consulted before the name
// switch — that is how an out-of-process tool declares its axis (M900).
func TestCapabilityFor_ManifestOverlayBeatsTheNameSwitch(t *testing.T) {
	k := &Kernel{toolCaps: map[string]edict.Capability{"shell": edict.CapMemory}}
	if got := k.capabilityFor(shellCall(), agent.ToolDef{Name: "shell"}); got != edict.CapMemory {
		t.Errorf("got %q, want the overlay's %q", got, edict.CapMemory)
	}
}

// And with neither, the name switch decides — including for the dynamic tool
// surfaces no static declaration can cover, since their ToolDefs are synthesized
// per attachment.
func TestCapabilityFor_NameSwitchStillCoversDynamicTools(t *testing.T) {
	k := &Kernel{}
	for _, tc := range []struct {
		name string
		want edict.Capability
	}{
		{"forge_summarize", edict.CapCodeExec},       // promoted script tool (M794)
		{"mcp_everything_echo", edict.CapMCP},        // bridged MCP call (M796)
		{"plug.post", edict.Capability("plug.post")}, // unmapped plugin tool, no manifest
	} {
		got := k.capabilityFor(agent.ToolCall{Name: tc.name, Input: json.RawMessage(`{}`)}, agent.ToolDef{Name: tc.name})
		if got != tc.want {
			t.Errorf("%s → %q, want %q", tc.name, got, tc.want)
		}
	}
}
