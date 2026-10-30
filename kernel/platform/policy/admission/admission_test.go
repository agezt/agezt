// SPDX-License-Identifier: MIT

package admission

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
)

// The ADMISSION rules' own tests (W3.75).
//
// The capability resolution and the approval bundle's tables lived in the
// daemon (kernel/runtime/policy.go and policy_helpers.go). These tests pin the
// behaviour where it now lives — the platform's rules, asked with the host's
// overlay and tool map.

func declaredDef(cap toolapi.ToolCapability) toolapi.ToolDef {
	return toolapi.ToolDef{Name: "probe", Capability: cap}
}

// TestTheValidatedOverlayKeepsOnlyKnownAxes: a plugin may join an existing
// policy axis, never invent one.
func TestTheValidatedOverlayKeepsOnlyKnownAxes(t *testing.T) {
	got := ValidatedToolCaps(map[string]string{"shell": string(edict.CapShell), "bogus": "not.a.capability"})
	if len(got) != 1 {
		t.Fatalf("the overlay must keep only known axes, got %+v", got)
	}
	if got["shell"] != edict.CapShell {
		t.Fatalf("shell = %q, want %q", got["shell"], edict.CapShell)
	}
	if got := ValidatedToolCaps(map[string]string{"bogus": "not.a.capability"}); got != nil {
		t.Fatalf("an overlay with nothing usable must be empty, got %+v", got)
	}
	if got := ValidatedToolCaps(nil); got != nil {
		t.Fatalf("an empty declaration must be empty, got %+v", got)
	}
}

// TestTheDeclarationWins: the tool's own declaration is the authoritative one.
func TestTheDeclarationWins(t *testing.T) {
	def := declaredDef(toolapi.ToolCapability{Name: string(edict.CapCodeExec)})
	got := ResolveCapability(def, map[string]edict.Capability{"probe": edict.CapMemory}, "probe", json.RawMessage(`{}`))
	if got != edict.CapCodeExec {
		t.Fatalf("capability = %q, want the declared %q", got, edict.CapCodeExec)
	}
}

// TestAnUnknownDeclarationIsIgnored: honouring a typo would resolve to an
// unknown capability, which is DEFAULT-DENIED — it would silently kill the
// tool. So the resolution continues down the list.
func TestAnUnknownDeclarationIsIgnored(t *testing.T) {
	def := declaredDef(toolapi.ToolCapability{Name: "shel"})
	got := ResolveCapability(def, nil, "shell", json.RawMessage(`{}`))
	if got != edict.CapShell {
		t.Fatalf("capability = %q, want the name switch's %q", got, edict.CapShell)
	}
}

// TestTheOverlayBeatsTheNameSwitch: the manifest overlay is the second source.
func TestTheOverlayBeatsTheNameSwitch(t *testing.T) {
	got := ResolveCapability(toolapi.ToolDef{Name: "shell"}, map[string]edict.Capability{"shell": edict.CapMemory}, "shell", json.RawMessage(`{}`))
	if got != edict.CapMemory {
		t.Fatalf("capability = %q, want the overlay's %q", got, edict.CapMemory)
	}
}

// TestTheNameSwitchCoversTheDynamicSurfaces: forged script tools and bridged
// MCP calls are synthesized per attachment, so no static declaration covers
// them.
func TestTheNameSwitchCoversTheDynamicSurfaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		want edict.Capability
	}{
		{"forge_summarize", edict.CapCodeExec},
		{"mcp_everything_echo", edict.CapMCP},
		{"plug.post", edict.Capability("plug.post")},
	} {
		got := ResolveCapability(toolapi.ToolDef{Name: tc.name}, nil, tc.name, json.RawMessage(`{}`))
		if got != tc.want {
			t.Fatalf("%s → %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestTheBundleUsesTheToolsOwnEffect: a declared effect wins.
func TestTheBundleUsesTheToolsOwnEffect(t *testing.T) {
	def := toolapi.ToolDef{
		Name: "probe",
		Effect: toolapi.ToolEffect{
			Class:             toolapi.EffectCompensable,
			PredictedEffects:  []string{"touch the probe"},
			AffectedResources: []string{"resource:probe"},
			RollbackNotes:     "undo the probe",
			Confidence:        0.42,
		},
	}
	bundle := BundleFor("probe", edict.CapShell, json.RawMessage(`{}`), def, nil)
	if bundle.EffectClass != string(toolapi.EffectCompensable) {
		t.Errorf("effect class = %q, want compensable", bundle.EffectClass)
	}
	if len(bundle.PredictedEffects) != 1 || bundle.PredictedEffects[0] != "touch the probe" {
		t.Errorf("predicted effects = %v, want the tool's own", bundle.PredictedEffects)
	}
	if len(bundle.AffectedResources) != 1 || bundle.AffectedResources[0] != "resource:probe" {
		t.Errorf("affected resources = %v, want the tool's own", bundle.AffectedResources)
	}
	if bundle.RollbackNotes != "undo the probe" {
		t.Errorf("rollback notes = %q, want the tool's own", bundle.RollbackNotes)
	}
	if bundle.Confidence != 0.42 {
		t.Errorf("confidence = %v, want the tool's own 0.42", bundle.Confidence)
	}
}

// TestTheBundleAsksTheHostForASilentDefinition: a tool that declares nothing
// is asked about through the lookup port.
//
// The looked-up effect must be DIFFERENT from the capability's own default,
// or a bundle that ignored the lookup would look the same.
func TestTheBundleAsksTheHostForASilentDefinition(t *testing.T) {
	asked := ""
	lookup := func(name string) (toolapi.ToolDef, bool) {
		asked = name
		return toolapi.ToolDef{
			Name: name,
			Effect: toolapi.ToolEffect{
				Class:             toolapi.EffectReadOnly,
				PredictedEffects:  []string{"the looked-up effect"},
				AffectedResources: []string{"resource:looked-up"},
				RollbackNotes:     "undo the looked-up effect",
				Confidence:        0.95,
			},
		}, true
	}
	bundle := BundleFor("probe", edict.CapShell, json.RawMessage(`{}`), toolapi.ToolDef{Name: "probe"}, lookup)
	if asked != "probe" {
		t.Fatalf("the lookup was asked for %q, want probe", asked)
	}
	if bundle.EffectClass != string(toolapi.EffectReadOnly) {
		t.Errorf("effect class = %q, want the looked-up read_only", bundle.EffectClass)
	}
	if bundle.Confidence != 0.95 {
		t.Errorf("confidence = %v, want the looked-up 0.95", bundle.Confidence)
	}
	if len(bundle.AffectedResources) != 1 || bundle.AffectedResources[0] != "resource:looked-up" {
		t.Errorf("affected resources = %v, want the looked-up one", bundle.AffectedResources)
	}
}

// TestTheBundleWithoutALookupUsesWhatItHas: a host with no tool map is not an
// error — the bundle falls back to the caller's own definition.
func TestTheBundleWithoutALookupUsesWhatItHas(t *testing.T) {
	bundle := BundleFor("probe", edict.CapFileRead, json.RawMessage(`{}`), toolapi.ToolDef{Name: "probe"}, nil)
	if bundle.EffectClass != string(toolapi.EffectReadOnly) {
		t.Fatalf("effect class = %q, want the capability's read_only", bundle.EffectClass)
	}
	if len(bundle.PredictedEffects) != 1 || !strings.Contains(bundle.PredictedEffects[0], "probe") {
		t.Errorf("predicted effects = %v, want the default for the call", bundle.PredictedEffects)
	}
	if bundle.RollbackNotes != "No rollback required for read-only action." {
		t.Fatalf("rollback notes = %q, want the read-only row's note", bundle.RollbackNotes)
	}
}

// TestTheBundleDerivesTheResourcesFromTheInput: when the tool declared none,
// the call's own input names them.
func TestTheBundleDerivesTheResourcesFromTheInput(t *testing.T) {
	bundle := BundleFor("probe", edict.CapFileWrite, json.RawMessage(`{"path":"/etc/hosts"}`), toolapi.ToolDef{Name: "probe"}, nil)
	found := false
	for _, r := range bundle.AffectedResources {
		if r == "path:/etc/hosts" {
			found = true
		}
	}
	if !found {
		t.Fatalf("affected resources = %v, want the input's path", bundle.AffectedResources)
	}
}

// TestTheBundleFallsBackToTheClassConfidence: a tool that declares no
// confidence is judged under its class's row — the platform's.
func TestTheBundleFallsBackToTheClassConfidence(t *testing.T) {
	bundle := BundleFor("probe", edict.CapFileRead, json.RawMessage(`{}`), toolapi.ToolDef{Name: "probe"}, nil)
	if bundle.Confidence != 0.95 {
		t.Fatalf("confidence = %v, want the read-only row's 0.95", bundle.Confidence)
	}
}

// TestTheEffectClassRows: the platform's table.
//
// The rows are a SLICE, not a map: a map's iteration order would make the
// failing row — and therefore the mutation log — vary from run to run.
func TestTheEffectClassRows(t *testing.T) {
	cases := []struct {
		cap  edict.Capability
		want string
	}{
		{edict.CapFileRead, string(toolapi.EffectReadOnly)},
		{edict.CapWebSearch, string(toolapi.EffectReadOnly)},
		{edict.CapFileWrite, string(toolapi.EffectReversible)},
		{edict.CapMemory, string(toolapi.EffectReversible)},
		{edict.CapNotify, string(toolapi.EffectCompensable)},
		{edict.CapHTTPPost, string(toolapi.EffectCompensable)},
		{edict.CapShell, string(toolapi.EffectIrreversible)},
		{edict.CapFileDelete, string(toolapi.EffectIrreversible)},
		{edict.CapCodeExec, string(toolapi.EffectIrreversible)},
		{edict.CapMarket, string(toolapi.EffectIrreversible)},
		{edict.Capability(""), string(toolapi.EffectIrreversible)},
	}
	for _, tc := range cases {
		if got := EffectClassFor(tc.cap); got != tc.want {
			t.Fatalf("the class of %q = %q, want %q", tc.cap, got, tc.want)
		}
	}
}

// TestTheNormalizedClasses: only the four governed classes are classes.
func TestTheNormalizedClasses(t *testing.T) {
	for _, c := range []toolapi.EffectClass{toolapi.EffectReadOnly, toolapi.EffectReversible, toolapi.EffectCompensable, toolapi.EffectIrreversible} {
		if got := NormalizeEffectClass(c); got != string(c) {
			t.Fatalf("normalized %q = %q, want itself", c, got)
		}
	}
	if got := NormalizeEffectClass(toolapi.EffectClass("nonsense")); got != "" {
		t.Fatalf("an unknown class must be dropped, got %q", got)
	}
	if got := NormalizeEffectClass(""); got != "" {
		t.Fatalf("an empty class must be dropped, got %q", got)
	}
}

// TestTheResourcesFromTheInput: the tool and the capability are always named,
// and the input's own keys are added.
func TestTheResourcesFromTheInput(t *testing.T) {
	got := AffectedResourcesFromInput("probe", edict.CapFileWrite, json.RawMessage(`{"path":"/tmp/x","url":"https://example.test"}`))
	want := []string{"tool:probe", "capability:file.write", "path:/tmp/x", "url:https://example.test"}
	if len(got) != len(want) {
		t.Fatalf("resources = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resources = %v, want %v", got, want)
		}
	}
}

// TestTheResourcesFromAnUnreadableInput: the two rows survive.
func TestTheResourcesFromAnUnreadableInput(t *testing.T) {
	got := AffectedResourcesFromInput("probe", edict.CapFileWrite, json.RawMessage(`not json`))
	if len(got) != 2 || got[0] != "tool:probe" || got[1] != "capability:file.write" {
		t.Fatalf("resources = %v, want the tool and the capability", got)
	}
}

// TestTheResourcesSkipEmptyValues: a blank key names nothing.
func TestTheResourcesSkipEmptyValues(t *testing.T) {
	got := AffectedResourcesFromInput("probe", edict.CapFileWrite, json.RawMessage(`{"path":"  ","op":"delete"}`))
	for _, r := range got {
		if strings.HasPrefix(r, "path:") {
			t.Fatalf("resources = %v, want the blank path skipped", got)
		}
	}
	if len(got) != 3 || got[2] != "op:delete" {
		t.Fatalf("resources = %v, want the op named", got)
	}
}

// TestTheRollbackRows: the platform's table.
func TestTheRollbackRows(t *testing.T) {
	cases := []struct {
		class string
		want  string
	}{
		{string(toolapi.EffectReadOnly), "No rollback required for read-only action."},
		{string(toolapi.EffectReversible), "Use the corresponding revert/delete/restore operation or journaled state to undo if needed."},
		{string(toolapi.EffectCompensable), "No guaranteed rollback; compensate with a follow-up action if the outcome is wrong."},
		{string(toolapi.EffectIrreversible), "No reliable rollback path declared; approve only if the effect is acceptable."},
		{"unknown", "No rollback information declared."},
	}
	for _, tc := range cases {
		if got := RollbackNotesFor(tc.class); got != tc.want {
			t.Fatalf("the rollback notes of %q = %q, want %q", tc.class, got, tc.want)
		}
	}
}
