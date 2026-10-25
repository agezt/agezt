// SPDX-License-Identifier: MIT

package workboardapi

import (
	"reflect"
	"testing"
)

// TestTheHostPortCarriesTheTwentyVerbs: the host-facing port is TWENTY verbs
// wide — the read side (Get, List) plus the eighteen lifecycle verbs the host
// drives.
//
// The width is the port's contract with every holder, and it is deliberately
// NOT the plugin's: the read port (`Store`) stays exactly two verbs wide,
// because a plugin that could claim, complete or archive a task would be
// driving another agent's work item. The host drives the whole lifecycle, so
// the host's port carries all twenty.
func TestTheHostPortCarriesTheTwentyVerbs(t *testing.T) {
	port := reflect.TypeOf((*HostStore)(nil)).Elem()
	if port.NumMethod() != 20 {
		t.Fatalf("the host port has %d verbs, want 20", port.NumMethod())
	}
	want := map[string]bool{
		"Get":                  true,
		"List":                 true,
		"Create":               true,
		"Claim":                true,
		"Heartbeat":            true,
		"Comment":              true,
		"Block":                true,
		"Fail":                 true,
		"Unblock":              true,
		"SetRetryPolicy":       true,
		"Complete":             true,
		"Prove":                true,
		"Review":               true,
		"SetSeat":              true,
		"Archive":              true,
		"Link":                 true,
		"AddDependency":        true,
		"BlockingDependencies": true,
		"ReclaimStale":         true,
		"SweepStaleClaims":     true,
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if !want[name] {
			t.Errorf("the host port grew an unexpected verb %q", name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("the host port lost the verb %q", name)
	}
}

// TestTheHostPortIsWiderThanTheReadPort: the host port carries the read side
// and more — a host port that DROPPED the read verbs would leave the plugin's
// port and the host's port unrelated, and the two shapes would drift.
//
// The read port's own width is pinned by the guard next door
// (TestTheStorePortIsTheReadSide); this one only says the host port is a
// SUPERSET of it.
func TestTheHostPortIsWiderThanTheReadPort(t *testing.T) {
	host := reflect.TypeOf((*HostStore)(nil)).Elem()
	read := reflect.TypeOf((*Store)(nil)).Elem()
	for i := 0; i < read.NumMethod(); i++ {
		m := read.Method(i)
		if _, ok := host.MethodByName(m.Name); !ok {
			t.Errorf("the host port does not carry the read verb %q", m.Name)
		}
	}
}

// TestTheRetryRuleIsTheContracts: the retry rule is PURE and it reads the
// task, so it lives in the contract layer with the task it reads. The rule
// decides whether a failed task retries, escalates or blocks — a second copy
// would drift, and the drift would show up as one task retrying forever next
// to another that gives up after a single failure.
func TestTheRetryRuleIsTheContracts(t *testing.T) {
	policy := &RetryPolicy{MaxAttempts: 3, EscalateTo: " lead "}

	// A budget that is not spent yet: retry, and say when.
	got := RetryDecisionFor(Task{RetryPolicy: policy, Attempts: []Attempt{{Status: "failed"}}}, "boom")
	if !got.Retry || got.Exhausted || got.NextAttempt != 2 || got.Action != "retry" {
		t.Fatalf("the first failure = %+v, want a retry on attempt 2", got)
	}
	if got.MaxAttempts != 3 || got.EscalateTo != "lead" {
		t.Fatalf("the decision lost the policy it read: %+v", got)
	}

	// A spent budget: escalate, and name the target.
	got = RetryDecisionFor(Task{RetryPolicy: policy, Attempts: []Attempt{{Status: "failed"}, {Status: "failed"}, {Status: "failed"}}}, "boom")
	if got.Retry || !got.Exhausted || got.Action != "escalate" {
		t.Fatalf("the third failure = %+v, want an escalation", got)
	}

	// No budget at all: block.
	got = RetryDecisionFor(Task{Attempts: []Attempt{{Status: "failed"}}}, "boom")
	if got.Retry || got.Exhausted || got.Action != "block" {
		t.Fatalf("a task without a policy = %+v, want a block", got)
	}

	// A policy that carries no budget is not a policy.
	if NormalizeRetryPolicy(&RetryPolicy{}) != nil {
		t.Fatal("an empty policy must normalize to nothing")
	}
	if NormalizeRetryPolicy(nil) != nil {
		t.Fatal("a missing policy must normalize to nothing")
	}
	normalized := NormalizeRetryPolicy(&RetryPolicy{MaxAttempts: 2, EscalateTo: " lead "})
	if normalized == nil || normalized.EscalateTo != "lead" {
		t.Fatalf("the escalation target was not trimmed: %+v", normalized)
	}
}
