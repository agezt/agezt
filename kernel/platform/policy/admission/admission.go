// SPDX-License-Identifier: MIT

// Package admission is the platform's ADMISSION rule: the policy axis a tool
// call is gated on, and what the operator is told about the call.
//
// It is the platform's for the same reason the guards are (W3.74): the rules
// own no store, they publish nothing, and every host that gates a tool call
// asks the same ones.
//
// The two rules are:
//
//   - the CAPABILITY RESOLUTION: which axis the call is judged on, in the
//     order the sources are specific — the tool's own declaration, the
//     plugin manifest overlay, and the name switch that still owns the
//     dynamic surfaces no static declaration can cover;
//   - the APPROVAL BUNDLE: the effect class, the predicted effects, the
//     affected resources, the rollback notes and the confidence the operator
//     is shown.
//
// The host supplies the one thing the platform cannot have: the tool map.
// The lookup is a PORT — a way to ASK for a definition by name.
package admission

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/policy/epistemic"
)

// ValidatedToolCaps keeps only declarations naming a capability Edict knows
// (M900). A plugin may join an existing policy axis, never invent one — an
// unknown capability is default-denied, so honouring a typo would silently
// kill the tool.
func ValidatedToolCaps(declared map[string]string) map[string]edict.Capability {
	if len(declared) == 0 {
		return nil
	}
	out := make(map[string]edict.Capability, len(declared))
	for tool, cap := range declared {
		if edict.KnownCapability(cap) {
			out[tool] = edict.Capability(cap)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ResolveCapability resolves the policy axis one tool call is gated on, in
// order of how specific the source is:
//
//  1. The tool's own ToolDef.Capability — the authoritative declaration, made
//     in the tool's package next to the behaviour it describes.
//  2. The manifest overlay (M900) — the same declaration arriving through a
//     plugin's capability manifest, for a tool whose ToolDef crossed a
//     process boundary and so cannot carry Go fields.
//  3. edict.CapabilityForToolCall — the name switch, now a fallback. It still
//     owns the DYNAMIC surfaces no static declaration can cover: forged
//     `forge_<name>` script tools and bridged `mcp_<server>_<tool>` calls.
//
// A declaration naming a capability Edict does not govern is IGNORED rather
// than honoured, and resolution continues down the list. This matters: an
// unknown capability is default-denied, so honouring a typo would silently
// kill the tool — the exact failure the overlay was introduced to end. The
// same rule the plugin manifest overlay already applied (a tool may join an
// existing axis, never invent one), now applied to in-tree declarations too.
func ResolveCapability(def toolapi.ToolDef, overlay map[string]edict.Capability, name string, input json.RawMessage) edict.Capability {
	if !def.Capability.IsZero() {
		if cap := def.Capability.For(input); edict.KnownCapability(cap) {
			return edict.Capability(cap)
		}
	}
	if cap, ok := overlay[name]; ok {
		return cap
	}
	return edict.CapabilityForToolCall(name, input)
}

// Bundle is what the operator is shown about a call: its effect class, the
// effects it predicts, the resources it touches, how it is rolled back and
// how sure the daemon is.
type Bundle struct {
	EffectClass       string
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

// Lookup finds a tool's definition by name — the host's tool map.
//
// A nil lookup means the host has no tool map: the bundle then works with
// the definition the caller handed it, and with nothing else.
type Lookup func(name string) (toolapi.ToolDef, bool)

// BundleFor assembles the bundle for one call.
//
// The tool's own declaration wins; when it says nothing at all, the host's
// tool map is asked for the definition instead.
func BundleFor(toolName string, cap edict.Capability, input json.RawMessage, def toolapi.ToolDef, lookup Lookup) Bundle {
	effect := def.Effect
	if effect.Class == "" && len(effect.PredictedEffects) == 0 && len(effect.AffectedResources) == 0 {
		if lookup != nil {
			if tool, ok := lookup(toolName); ok {
				effect = tool.Effect
			}
		}
	}

	class := NormalizeEffectClass(effect.Class)
	if class == "" {
		class = EffectClassFor(cap)
	}
	resources := append([]string(nil), effect.AffectedResources...)
	if len(resources) == 0 {
		resources = AffectedResourcesFromInput(toolName, cap, input)
	}
	predicted := append([]string(nil), effect.PredictedEffects...)
	if len(predicted) == 0 {
		predicted = []string{fmt.Sprintf("invoke %s under %s", toolName, cap)}
	}
	rollback := strings.TrimSpace(effect.RollbackNotes)
	if rollback == "" {
		rollback = RollbackNotesFor(class)
	}
	confidence := effect.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = epistemic.DefaultEffectConfidence(class)
	}
	return Bundle{
		EffectClass:       class,
		PredictedEffects:  predicted,
		AffectedResources: resources,
		RollbackNotes:     rollback,
		Confidence:        confidence,
	}
}

// NormalizeEffectClass keeps the four effect classes the platform governs and
// drops everything else (an unknown class is not a class).
func NormalizeEffectClass(class toolapi.EffectClass) string {
	switch class {
	case toolapi.EffectReadOnly, toolapi.EffectReversible, toolapi.EffectCompensable, toolapi.EffectIrreversible:
		return string(class)
	default:
		return ""
	}
}

// EffectClassFor is the platform's row: the effect class a capability
// carries when the tool declared none.
//
// The default is the SAFE one — an axis nobody classified is treated as
// irreversible, which asks rather than assumes.
func EffectClassFor(cap edict.Capability) string {
	switch cap {
	case edict.CapFileRead, edict.CapFileList, edict.CapHTTPGet, edict.CapBrowserRead,
		edict.CapHomeAssistantRead, edict.CapWebSearch, edict.CapRunsRead,
		edict.CapIntrospect, edict.CapConfigRead, edict.CapProviderCall:
		return string(toolapi.EffectReadOnly)
	case edict.CapFileWrite, edict.CapMemory, edict.CapWorld, edict.CapSchedule,
		edict.CapStanding, edict.CapBoard, edict.CapSkill, edict.CapOversee,
		edict.CapToolForge, edict.CapConfigWrite, edict.CapWorkflow:
		return string(toolapi.EffectReversible)
	case edict.CapNotify, edict.CapHTTPPost, edict.CapRemoteRun:
		return string(toolapi.EffectCompensable)
	case edict.CapShell, edict.CapFileDelete, edict.CapCoding, edict.CapACPAgent,
		edict.CapHomeAssistantCall, edict.CapCodeExec, edict.CapMCPInstall, edict.CapMCP,
		edict.CapMarket:
		return string(toolapi.EffectIrreversible)
	default:
		return string(toolapi.EffectIrreversible)
	}
}

// AffectedResourcesFromInput derives the resources a call touches from its
// input, when the tool declared none.
func AffectedResourcesFromInput(toolName string, cap edict.Capability, input json.RawMessage) []string {
	out := []string{"tool:" + toolName, "capability:" + string(cap)}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return out
	}
	for _, key := range []string{"path", "url", "endpoint", "entity_id", "service", "command", "op", "name", "id"} {
		if v, ok := obj[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, key+":"+s)
			}
		}
	}
	return out
}

// RollbackNotesFor is the platform's row: what the operator is told about
// undoing a call of this class.
func RollbackNotesFor(class string) string {
	switch class {
	case string(toolapi.EffectReadOnly):
		return "No rollback required for read-only action."
	case string(toolapi.EffectReversible):
		return "Use the corresponding revert/delete/restore operation or journaled state to undo if needed."
	case string(toolapi.EffectCompensable):
		return "No guaranteed rollback; compensate with a follow-up action if the outcome is wrong."
	case string(toolapi.EffectIrreversible):
		return "No reliable rollback path declared; approve only if the effect is acceptable."
	default:
		return "No rollback information declared."
	}
}
