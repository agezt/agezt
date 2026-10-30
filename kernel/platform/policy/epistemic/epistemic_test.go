// SPDX-License-Identifier: MIT

package epistemic

// The EPISTEMIC GUARD's own tests (W3.73).
//
// The guard lived in the daemon (kernel/runtime/epistemic.go and
// epistemic_helpers.go, 325 lines): it read the journal's memory, weighed the
// failures it found and decided whether a tool call needs a human. These
// tests pin the behaviour where it now lives — the platform's rule, asked
// through the host's ports.
//
// The rule's shape: a tool the journal has never seen is NOVEL, a tool that
// failed under the same conditions is SUSPECT, and an effectful call with a
// permissive schema behind a dynamic surface is both.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// fixedNow is the clock the history is measured against.
var fixedNow = time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

// journalEvent builds one journaled entry.
func journalEvent(kind event.Kind, ts time.Time, payload map[string]any) *event.Event {
	raw, _ := json.Marshal(payload)
	return &event.Event{Kind: kind, TSUnixMS: ts.UnixMilli(), Payload: raw}
}

// policyDecision is a journaled policy decision.
func policyDecision(tool, callID, cap, schemaHash, shape string, ts time.Time) *event.Event {
	return journalEvent(event.KindPolicyDecision, ts, map[string]any{
		"tool": tool, "call_id": callID, "capability": cap,
		"schema_hash": schemaHash, "input_shape": shape,
	})
}

// toolResult is a journaled tool result.
func toolResult(tool, callID string, failed bool, ts time.Time) *event.Event {
	return journalEvent(event.KindToolResult, ts, map[string]any{
		"tool": tool, "call_id": callID, "error": failed,
	})
}

// scanner turns a list of events into the host's scan port.
func scanner(events ...*event.Event) func(func(*event.Event) error) error {
	return func(visit func(*event.Event) error) error {
		for _, e := range events {
			if err := visit(e); err != nil {
				return err
			}
		}
		return nil
	}
}

// effectfulDef is a tool that changes something, with a declared schema.
func effectfulDef() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:        "shell",
		Description: "runs a command",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`),
		Effect:      toolapi.ToolEffect{Class: toolapi.EffectCompensable},
	}
}

// TestTheGuardAllowsAnUnknownTool: nothing is known, so the guard says so
// (the tool is novel) — but novelty alone is not a reason to stop a call the
// normal policy already allowed.
func TestTheGuardAllowsAnUnknownTool(t *testing.T) {
	gate := New(Ports{Scan: scanner(), Now: func() time.Time { return fixedNow }})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	if decision.Escalates() {
		t.Fatalf("an unknown tool must not escalate on its own: %+v", decision)
	}
	if !decision.NovelTool {
		t.Fatal("a tool the journal never saw must be novel")
	}
	if !containsSignal(decision.Signals, "novel_tool_conditions") {
		t.Fatalf("the novelty signal is missing: %v", decision.Signals)
	}
	if decision.Confidence != 0.9 {
		t.Fatalf("the decision must keep the call's confidence, got %v", decision.Confidence)
	}
}

// TestTheGuardEscalatesALowConfidence: an effectful call whose confidence
// sits below its class's floor goes to a human.
func TestTheGuardEscalatesALowConfidence(t *testing.T) {
	gate := New(Ports{Scan: scanner(), Now: func() time.Time { return fixedNow }})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.1)
	if !decision.Escalates() {
		t.Fatalf("a low-confidence effectful call must escalate: %+v", decision)
	}
	if !containsSignal(decision.Signals, "low_effect_confidence") {
		t.Fatalf("the low-confidence signal is missing: %v", decision.Signals)
	}
}

// TestTheGuardEscalatesAPermissiveSchemaBehindADynamicSurface: an effectful
// tool that accepts anything, behind a surface the model picked at runtime,
// is the combination a human must see.
func TestTheGuardEscalatesAPermissiveSchemaBehindADynamicSurface(t *testing.T) {
	gate := New(Ports{Scan: scanner(), Now: func() time.Time { return fixedNow }})
	def := toolapi.ToolDef{Name: "mcp_fake_greet", Description: "greets", InputSchema: json.RawMessage(`{"type":"object"}`)}
	decision := gate.Decide("mcp_fake_greet", "fs.write", json.RawMessage(`{}`), def, string(toolapi.EffectCompensable), 0.9)
	if !decision.Escalates() {
		t.Fatalf("a permissive schema behind a dynamic surface must escalate: %+v", decision)
	}
	if !containsSignal(decision.Signals, "dynamic_tool_surface") ||
		!containsSignal(decision.Signals, "permissive_schema_effectful_tool") {
		t.Fatalf("the signals do not name the combination: %v", decision.Signals)
	}
}

// TestTheGuardEscalatesAMatchingFailure: the journal remembers a failure
// under the same conditions — that is the memory the guard exists for.
func TestTheGuardEscalatesAMatchingFailure(t *testing.T) {
	shape := inputShape(json.RawMessage(`{"cmd":"ls"}`))
	hash := hashSchema(effectfulDef().InputSchema)
	events := scanner(
		policyDecision("shell", "call-1", "fs.write", hash, shape, fixedNow.Add(-time.Minute)),
		toolResult("shell", "call-1", true, fixedNow.Add(-time.Minute)),
	)
	gate := New(Ports{Scan: events, Now: func() time.Time { return fixedNow }})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	if !decision.Escalates() {
		t.Fatalf("a matching failure must escalate: %+v", decision)
	}
	if decision.FailureMatches != 1 {
		t.Fatalf("the decision must count the matching failures, got %d", decision.FailureMatches)
	}
	if decision.NovelTool {
		t.Fatal("a tool the journal has seen must not be novel")
	}
	if !containsSignal(decision.Signals, "matched_failure_conditions") {
		t.Fatalf("the matched-failure signal is missing: %v", decision.Signals)
	}
}

// TestTheFailureWeightDecays: a failure from last week says much less about
// today than one from an hour ago.
func TestTheFailureWeightDecays(t *testing.T) {
	shape := inputShape(json.RawMessage(`{"cmd":"ls"}`))
	hash := hashSchema(effectfulDef().InputSchema)
	fresh := New(Ports{
		Scan: scanner(
			policyDecision("shell", "call-1", "fs.write", hash, shape, fixedNow.Add(-time.Minute)),
			toolResult("shell", "call-1", true, fixedNow.Add(-time.Minute)),
		),
		Now: func() time.Time { return fixedNow },
	})
	old := New(Ports{
		Scan: scanner(
			policyDecision("shell", "call-1", "fs.write", hash, shape, fixedNow.Add(-7*24*time.Hour)),
			toolResult("shell", "call-1", true, fixedNow.Add(-7*24*time.Hour)),
		),
		Now: func() time.Time { return fixedNow },
	})
	freshDecision := fresh.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	oldDecision := old.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	if freshDecision.WeightedFailures <= oldDecision.WeightedFailures {
		t.Fatalf("an old failure must weigh less: %v vs %v", oldDecision.WeightedFailures, freshDecision.WeightedFailures)
	}
	if freshDecision.Escalates() != oldDecision.Escalates() {
		// Both fail under the same conditions; the weight decides how many
		// it takes, not whether the first one counts.
		t.Logf("the fresh and old verdicts differ: %v vs %v", freshDecision.Escalates(), oldDecision.Escalates())
	}
}

// TestAnOldFailureDoesNotEscalate: a failure from last week is still
// remembered, but it no longer carries enough weight to stop a call — the
// memory decays with the failure's age.
func TestAnOldFailureDoesNotEscalate(t *testing.T) {
	shape := inputShape(json.RawMessage(`{"cmd":"ls"}`))
	hash := hashSchema(effectfulDef().InputSchema)
	gate := New(Ports{
		Scan: scanner(
			policyDecision("shell", "call-1", "fs.write", hash, shape, fixedNow.Add(-7*24*time.Hour)),
			toolResult("shell", "call-1", true, fixedNow.Add(-7*24*time.Hour)),
		),
		Now: func() time.Time { return fixedNow },
	})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	if decision.FailureMatches != 1 {
		t.Fatalf("the old failure must still be counted, got %d", decision.FailureMatches)
	}
	if decision.WeightedFailures >= 0.75 {
		t.Fatalf("the old failure must weigh less than the bar, got %v", decision.WeightedFailures)
	}
	if decision.Escalates() {
		t.Fatalf("a week-old failure must not stop a call: %+v", decision)
	}
}

// TestTheGuardIgnoresAFailureUnderOtherConditions: the memory is about the
// CALL, not about the tool's name.
func TestTheGuardIgnoresAFailureUnderOtherConditions(t *testing.T) {
	hash := hashSchema(effectfulDef().InputSchema)
	events := scanner(
		policyDecision("shell", "call-1", "fs.write", hash, "object{other:string}", fixedNow.Add(-time.Minute)),
		toolResult("shell", "call-1", true, fixedNow.Add(-time.Minute)),
	)
	gate := New(Ports{Scan: events, Now: func() time.Time { return fixedNow }})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	if decision.FailureMatches != 0 {
		t.Fatalf("a failure under other conditions must not count: %+v", decision)
	}
}

// TestAReadOnlyToolEscalatesOnlyOnRepeatedFailures: reading is cheap, so
// one failure is not a reason to stop it.
func TestAReadOnlyToolEscalatesOnlyOnRepeatedFailures(t *testing.T) {
	def := toolapi.ToolDef{Name: "read", Description: "reads a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}
	shape := inputShape(json.RawMessage(`{"path":"a"}`))
	hash := hashSchema(def.InputSchema)
	one := New(Ports{
		Scan: scanner(
			policyDecision("read", "call-1", "fs.read", hash, shape, fixedNow.Add(-time.Minute)),
			toolResult("read", "call-1", true, fixedNow.Add(-time.Minute)),
		),
		Now: func() time.Time { return fixedNow },
	})
	decision := one.Decide("read", "fs.read", json.RawMessage(`{"path":"a"}`), def, string(toolapi.EffectReadOnly), 0.95)
	if decision.Escalates() {
		t.Fatalf("one failure must not stop a read-only call: %+v", decision)
	}
	two := New(Ports{
		Scan: scanner(
			policyDecision("read", "call-1", "fs.read", hash, shape, fixedNow),
			toolResult("read", "call-1", true, fixedNow),
			policyDecision("read", "call-2", "fs.read", hash, shape, fixedNow),
			toolResult("read", "call-2", true, fixedNow),
		),
		Now: func() time.Time { return fixedNow },
	})
	decision = two.Decide("read", "fs.read", json.RawMessage(`{"path":"a"}`), def, string(toolapi.EffectReadOnly), 0.95)
	if !decision.Escalates() {
		t.Fatalf("repeated failures must stop a read-only call: %+v", decision)
	}
}

// TestTheGuardLooksTheToolUpWhenTheCallerHasNoDefinition: the host's tool
// map is the gate's way to learn what a tool is.
func TestTheGuardLooksTheToolUpWhenTheCallerHasNoDefinition(t *testing.T) {
	gate := New(Ports{
		Scan: scanner(),
		Now:  func() time.Time { return fixedNow },
		Lookup: func(name string) (toolapi.ToolDef, bool) {
			if name == "shell" {
				return effectfulDef(), true
			}
			return toolapi.ToolDef{}, false
		},
	})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), toolapi.ToolDef{}, string(toolapi.EffectCompensable), 0.9)
	if decision.SchemaHash == "" {
		t.Fatalf("the looked-up definition must shape the decision: %+v", decision)
	}
	if !decision.NovelTool {
		t.Fatal("the journal knows nothing about this tool, so it is novel")
	}
}

// TestTheGuardAllowsACallWithNoDefinition: a call the caller cannot describe
// is handled before the gate — the gate must not invent a verdict.
func TestTheGuardAllowsACallWithNoDefinition(t *testing.T) {
	gate := New(Ports{Scan: scanner(), Now: func() time.Time { return fixedNow }})
	decision := gate.Decide("ghost", "fs.write", json.RawMessage(`{}`), toolapi.ToolDef{}, string(toolapi.EffectCompensable), 0.9)
	if decision.Escalates() {
		t.Fatalf("a call with no definition must not escalate: %+v", decision)
	}
	if !strings.Contains(decision.Reason, "tool definition unavailable") {
		t.Fatalf("the reason must say why: %q", decision.Reason)
	}
}

// TestTheGuardReadsTheTemporalSignal: a call that reads the world's time is
// marked, because its meaning decays.
func TestTheGuardReadsTheTemporalSignal(t *testing.T) {
	gate := New(Ports{Scan: scanner(), Now: func() time.Time { return fixedNow }})
	def := toolapi.ToolDef{Name: "weather", Description: "today's weather", InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}
	decision := gate.Decide("weather", "fs.read", json.RawMessage(`{"city":"Izmir"}`), def, string(toolapi.EffectReadOnly), 0.95)
	if !decision.Temporal {
		t.Fatalf("the temporal signal is missing: %+v", decision)
	}
}

// TestTheGuardWithoutAJournalKnowsNothing: a host with no journal hands the
// guard no memory — the guard must not invent one.
func TestTheGuardWithoutAJournalKnowsNothing(t *testing.T) {
	gate := New(Ports{})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0.9)
	if decision.FailureMatches != 0 || decision.WeightedFailures != 0 {
		t.Fatalf("no journal means no memory: %+v", decision)
	}
	if !decision.NovelTool {
		t.Fatal("no journal means every tool is novel")
	}
}

// TestTheDefaultConfidenceIsTheClassesOwn: the platform's row — the
// runtime's approval bundle and the guard read the same numbers.
func TestTheDefaultConfidenceIsTheClassesOwn(t *testing.T) {
	cases := map[string]float64{
		string(toolapi.EffectReadOnly):     0.95,
		string(toolapi.EffectReversible):   0.75,
		string(toolapi.EffectCompensable):  0.6,
		string(toolapi.EffectIrreversible): 0.5,
		"unknown":                          0.4,
	}
	for class, want := range cases {
		if got := DefaultEffectConfidence(class); got != want {
			t.Fatalf("the default confidence for %q is %v, want %v", class, got, want)
		}
	}
}

// TestTheGuardFallsBackToTheDefaultConfidence: a call with no declared
// confidence is judged under its class's default.
func TestTheGuardFallsBackToTheDefaultConfidence(t *testing.T) {
	gate := New(Ports{Scan: scanner(), Now: func() time.Time { return fixedNow }})
	decision := gate.Decide("shell", "fs.write", json.RawMessage(`{"cmd":"ls"}`), effectfulDef(), string(toolapi.EffectCompensable), 0)
	if decision.Confidence != DefaultEffectConfidence(string(toolapi.EffectCompensable)) {
		t.Fatalf("the decision must fall back to the class default, got %v", decision.Confidence)
	}
}
