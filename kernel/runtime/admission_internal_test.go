// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/policy/admission"
)

// The admission rules moved to the platform in W3.75. These tests pin the
// daemon's half of the move: the daemon ASKS the platform's rules over its
// own overlay and tool map, and it does not grow its own copy of them.

type admissionHostTool struct {
	def toolapi.ToolDef
}

func (t admissionHostTool) Definition() toolapi.ToolDef { return t.def }

func (t admissionHostTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "ok"}, nil
}

// TestTheCapabilityResolutionIsThePlatformsRule pins the delegation: the
// daemon's answer is the platform's answer over the same overlay.
func TestTheCapabilityResolutionIsThePlatformsRule(t *testing.T) {
	overlay := map[string]edict.Capability{"shell": edict.CapMemory}
	k := &Kernel{toolCaps: overlay}
	tc := llm.ToolCall{Name: "shell", Input: json.RawMessage(`{"command":"echo hi"}`)}
	def := toolapi.ToolDef{Name: "shell"}

	got := k.capabilityFor(tc, def)
	want := admission.ResolveCapability(def, overlay, tc.Name, tc.Input)
	if got != want {
		t.Fatalf("capability = %q, want the platform's %q", got, want)
	}
	if got != edict.CapMemory {
		t.Fatalf("capability = %q, want the overlay's %q", got, edict.CapMemory)
	}
}

// TestTheApprovalBundleIsThePlatformsRule pins the second delegation: the
// bundle the operator is shown is the platform's bundle over the same tool
// map.
func TestTheApprovalBundleIsThePlatformsRule(t *testing.T) {
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
	k := &Kernel{tools: map[string]toolapi.Tool{"probe": admissionHostTool{def: def}}}
	input := json.RawMessage(`{"target":"prod"}`)

	got := k.approvalDecisionBundle("probe", edict.CapShell, input, toolapi.ToolDef{Name: "probe"})
	want := admission.BundleFor("probe", edict.CapShell, input, toolapi.ToolDef{Name: "probe"}, k.lookupToolDef)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle = %+v, want the platform's %+v", got, want)
	}
	if got.EffectClass != string(toolapi.EffectCompensable) {
		t.Fatalf("effect class = %q, want the tool's own compensable", got.EffectClass)
	}
	if got.Confidence != 0.42 {
		t.Fatalf("confidence = %v, want the tool's own 0.42", got.Confidence)
	}
}

// TestTheAdmissionRulesLiveInThePlatform is the ratchet: the daemon's policy
// hook must not creep the admission rules back. The rules are the platform's
// (kernel/platform/policy/admission); the daemon keeps the doors.
func TestTheAdmissionRulesLiveInThePlatform(t *testing.T) {
	for _, file := range []string{"policy.go", "policy_helpers.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(src)
		for _, forbidden := range []string{
			"edict.KnownCapability(cap)",
			"edict.CapabilityForToolCall(",
			"func defaultEffectClass(",
			"func defaultRollbackNotes(",
			"func normalizeEffectClass(",
			"func affectedResourcesFromInput(",
			"type approvalBundle struct",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s grew the admission rule %q back; the rules are the platform's", file, forbidden)
			}
		}
	}
}
