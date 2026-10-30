// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/platform/policy/epistemic"
)

// The epistemic guard moved to the platform in W3.73. These tests pin the
// RUNTIME's half of the move: the kernel keeps the ports (journal, clock,
// tool map) and asks the platform's rule — it does not decide on its own.

type epistemicHostTool struct {
	def toolapi.ToolDef
}

func (t epistemicHostTool) Definition() toolapi.ToolDef { return t.def }

func (t epistemicHostTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "ok"}, nil
}

func epistemicHostDef() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:        "riskprobe",
		Description: "a host tool",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"additionalProperties":false}`),
		Effect: toolapi.ToolEffect{
			Class:      toolapi.EffectCompensable,
			Confidence: 0.95,
		},
	}
}

func epistemicHostKernel(j *journal.Journal) *Kernel {
	return &Kernel{
		journal: j,
		tools: map[string]toolapi.Tool{
			"riskprobe": epistemicHostTool{def: epistemicHostDef()},
		},
	}
}

// TestEpistemicGateAsksThePlatformRule pins the delegation: the kernel's
// decision is the platform's decision over the same ports, and the rule it
// applies is the platform's (a low-confidence compensable call escalates).
func TestEpistemicGateAsksThePlatformRule(t *testing.T) {
	k := epistemicHostKernel(nil)
	input := json.RawMessage(`{"target":"prod"}`)
	bundle := approvalBundle{EffectClass: string(toolapi.EffectCompensable), Confidence: 0.3}

	got := k.epistemicGate("riskprobe", edict.CapFileWrite, input, toolapi.ToolDef{}, bundle)

	want := k.epistemicGateFor().Decide(
		"riskprobe",
		string(edict.CapFileWrite),
		input,
		toolapi.ToolDef{},
		string(toolapi.EffectCompensable),
		0.3,
	)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kernel decision = %+v, want the platform's %+v", got, want)
	}
	if !got.Escalates() {
		t.Fatalf("decision = %+v, want an escalation (confidence 0.3 < the compensable floor)", got)
	}
	if got.Confidence != 0.3 {
		t.Fatalf("confidence = %v want 0.3", got.Confidence)
	}
}

// TestEpistemicGateResolvesTheToolThroughTheLookupPort pins the second port:
// the kernel finds the tool's definition in its own tool map when the caller
// hands the guard none.
func TestEpistemicGateResolvesTheToolThroughTheLookupPort(t *testing.T) {
	k := epistemicHostKernel(nil)
	input := json.RawMessage(`{"target":"prod"}`)
	bundle := approvalBundle{EffectClass: string(toolapi.EffectCompensable), Confidence: 0.95}

	got := k.epistemicGate("riskprobe", edict.CapFileWrite, input, toolapi.ToolDef{}, bundle)

	if got.SchemaHash == "" {
		t.Fatalf("schema hash = %q, want the definition the lookup port resolved", got.SchemaHash)
	}
	if !strings.Contains(got.InputShape, "target:string") {
		t.Fatalf("input shape = %q, want the call's shape", got.InputShape)
	}
}

// TestEpistemicGateReadsTheJournalThroughTheScanPort pins the third port:
// the kernel's journal is the history the platform's rule reasons over — a
// journaled failure under the call's conditions is matched.
func TestEpistemicGateReadsTheJournalThroughTheScanPort(t *testing.T) {
	j, err := journal.Open(filepath.Join(t.TempDir(), "journal"), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	k := epistemicHostKernel(j)

	input := json.RawMessage(`{"target":"prod"}`)
	bundle := approvalBundle{EffectClass: string(toolapi.EffectCompensable), Confidence: 0.95}

	// The first decision carries the conditions the journal must record for
	// the second one to match.
	first := k.epistemicGate("riskprobe", edict.CapFileWrite, input, toolapi.ToolDef{}, bundle)
	if first.FailureMatches != 0 {
		t.Fatalf("first decision matched %d failures, want 0", first.FailureMatches)
	}

	if _, err := j.Append(event.Spec{
		Subject: "agent.01/tool",
		Kind:    event.KindPolicyDecision,
		Actor:   "kernel",
		Payload: map[string]any{
			"tool":        "riskprobe",
			"call_id":     "c1",
			"capability":  string(edict.CapFileWrite),
			"schema_hash": first.SchemaHash,
			"input_shape": first.InputShape,
		},
	}); err != nil {
		t.Fatalf("append policy decision: %v", err)
	}
	if _, err := j.Append(event.Spec{
		Subject: "agent.01/tool",
		Kind:    event.KindToolResult,
		Actor:   "kernel",
		Payload: map[string]any{
			"tool":    "riskprobe",
			"call_id": "c1",
			"error":   true,
		},
	}); err != nil {
		t.Fatalf("append tool result: %v", err)
	}

	second := k.epistemicGate("riskprobe", edict.CapFileWrite, input, toolapi.ToolDef{}, bundle)
	if second.FailureMatches != 1 {
		t.Fatalf("failure matches = %d want 1 (the journal's failure under the call's conditions)", second.FailureMatches)
	}
	if second.WeightedFailures <= 0 {
		t.Fatalf("weighted failures = %v want > 0", second.WeightedFailures)
	}
	if second.NovelTool {
		t.Fatal("novel tool = true, want false (the journal has seen this tool)")
	}
	if !containsString(second.Signals, "matched_failure_conditions:1") {
		t.Fatalf("signals = %v, want the matched failure", second.Signals)
	}
	if !second.Escalates() {
		t.Fatalf("decision = %+v, want an escalation (a matched failure)", second)
	}
}

// TestEpistemicGateWithoutAJournalKnowsNothing pins the safe direction: a
// kernel with no journal knows nothing, which makes the tool novel — and
// unknown means ask.
func TestEpistemicGateWithoutAJournalKnowsNothing(t *testing.T) {
	k := epistemicHostKernel(nil)
	input := json.RawMessage(`{"target":"prod"}`)
	bundle := approvalBundle{EffectClass: string(toolapi.EffectReadOnly), Confidence: 0.95}

	got := k.epistemicGate("riskprobe", edict.CapFileRead, input, toolapi.ToolDef{}, bundle)

	if got.FailureMatches != 0 || got.WeightedFailures != 0 {
		t.Fatalf("failure counts = %d/%v want 0/0", got.FailureMatches, got.WeightedFailures)
	}
	if !got.NovelTool {
		t.Fatal("novel tool = false, want true (no journal means no memory)")
	}
	if !containsString(got.Signals, "novel_tool_conditions") {
		t.Fatalf("signals = %v, want the novel signal", got.Signals)
	}
}

// TestEpistemicDefaultEffectConfidenceIsThePlatformRow pins that the runtime
// and the platform share ONE confidence row: the approval bundle's fallback
// confidence is the platform's value.
func TestEpistemicDefaultEffectConfidenceIsThePlatformRow(t *testing.T) {
	if got := epistemic.DefaultEffectConfidence(string(toolapi.EffectCompensable)); got != 0.6 {
		t.Fatalf("compensable confidence = %v want 0.6", got)
	}
}

func containsString(signals []string, want string) bool {
	for _, s := range signals {
		if s == want {
			return true
		}
	}
	return false
}
