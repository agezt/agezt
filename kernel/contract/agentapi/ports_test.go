// SPDX-License-Identifier: MIT

package agentapi_test

// Provenance: the contract's own tests for the W3.56 slice — the widths of
//             the roster's ports, the exact verbs they carry, the rule that
//             travels with the profile row (the autonomy runbook), and the
//             sentinels the layers compare against.
//
// The widths are checked STATICALLY, on the interface types: a narrowed port
// would still answer the read side, so only the declared method set can tell
// the two apart.

import (
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// TestTheStorePortIsExactlyEightVerbsWide: the store is the module's WHOLE
// public store surface as the layers above it see it — and no more.
//
// The width is a promise in both directions: too narrow and the layers above
// cannot do their work, too wide and the port grows verbs nobody calls (and
// every fake has to implement them).
func TestTheStorePortIsExactlyEightVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*agentapi.Store)(nil)).Elem()
	if got := typ.NumMethod(); got != 8 {
		t.Fatalf("the store port has %d verbs, want 8", got)
	}
	want := []string{"Add", "Count", "Get", "List", "Remove", "SetEnabled", "SetRetired", "Update"}
	for _, name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the store port is missing %s", name)
		}
	}
}

// TestTheStoreNamesOnlyContractRows: the store's exact signatures. A port
// that answers a row the contract does not have is a port the contract layer
// cannot describe — and a shape the modules above would have to reach back
// into the implementation for.
func TestTheStoreNamesOnlyContractRows(t *testing.T) {
	want := map[string]func(){
		"Add":        func() { var p agentapi.Store; _, _ = p.Add(agentapi.Profile{}) },
		"SetEnabled": func() { var p agentapi.Store; _, _ = p.SetEnabled("", false) },
		"SetRetired": func() { var p agentapi.Store; _, _ = p.SetRetired("", false) },
		"Update":     func() { var p agentapi.Store; _, _ = p.Update("", func(*agentapi.Profile) {}) },
		"Remove":     func() { var p agentapi.Store; _, _, _ = p.Remove("") },
		"Get":        func() { var p agentapi.Store; _, _ = p.Get("") },
		"List":       func() { var p agentapi.Store; _ = p.List() },
		"Count":      func() { var p agentapi.Store; _ = p.Count() },
	}
	typ := reflect.TypeOf((*agentapi.Store)(nil)).Elem()
	for name := range want {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the store port is missing %s", name)
		}
	}
	if typ.NumMethod() != len(want) {
		t.Fatalf("the store port has %d verbs, want %d", typ.NumMethod(), len(want))
	}
	// No verb takes a time.Time: the store stamps the rows from its own clock,
	// so the contract layer never has to name one (principle P4).
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		for j := 0; j < m.Type.NumIn(); j++ {
			if m.Type.In(j) == reflect.TypeOf(time.Time{}) {
				t.Errorf("%s takes a time.Time — the contract may not name a clock", m.Name)
			}
		}
	}
}

// TestTheReaderPortIsExactlyTwoVerbsWide: the read-only half of the roster.
// A reader that could also write would hand every read-only consumer — the
// agent gateway, the web UI's agent list — the whole lifecycle.
func TestTheReaderPortIsExactlyTwoVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*agentapi.Reader)(nil)).Elem()
	if got := typ.NumMethod(); got != 2 {
		t.Fatalf("the reader port has %d verbs, want 2", got)
	}
	for _, name := range []string{"Get", "List"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the reader port is missing %s", name)
		}
	}
}

// TestTheRepairSourcePortIsOneVerb: a repair pass is a governed run of the
// target agent that may change its durable profile and reports what it
// changed. One verb, one meaning.
func TestTheRepairSourcePortIsOneVerb(t *testing.T) {
	typ := reflect.TypeOf((*agentapi.RepairSource)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the repair-source port has %d verbs, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("RepairAgent")
	if !ok {
		t.Fatal("the repair-source port has no RepairAgent")
	}
	if got, want := m.Type.NumIn(), 2; got != want {
		t.Fatalf("RepairAgent takes %d values, want %d (the ref and the reason)", got, want)
	}
	if got, want := m.Type.NumOut(), 2; got != want {
		t.Fatalf("RepairAgent answers %d values, want %d (the result and the error)", got, want)
	}
}

// TestTheSentinelsAreTheContracts: the sentinels the layers compare against
// live in the contract, so `errors.Is` matches them wherever they are raised.
func TestTheSentinelsAreTheContracts(t *testing.T) {
	for name, err := range map[string]error{
		"ErrNotFound": agentapi.ErrNotFound,
		"ErrRetired":  agentapi.ErrRetired,
	} {
		if err == nil {
			t.Errorf("%s is nil", name)
			continue
		}
		if err.Error() == "" {
			t.Errorf("%s has no message", name)
		}
	}
	// The two sentinels are distinct — a layer that compares against the
	// wrong one would misreport the reason ("not found" for a retired agent).
	if agentapi.ErrNotFound == agentapi.ErrRetired {
		t.Fatal("the roster sentinels must be distinct")
	}
}

// TestTheAutonomyRunbookTravelsWithTheRow: the runbook IS the profile's
// trigger/route/recovery/sleep behaviour. A rule that quietly returned a
// different shape would leave every wake evidence (operator wakes, schedule
// fires, standing fires, delegated spawns) carrying a different contract —
// and no structural check would see it.
func TestTheAutonomyRunbookTravelsWithTheRow(t *testing.T) {
	// A plain, direct-callable, enabled agent: the operator schedule channel
	// triggers it, it owns itself, recovery is manual, it never sleeps.
	// (DirectCallable unset means direct-callable — the contract's default.)
	base := agentapi.Profile{Slug: "ops", Name: "Ops", Enabled: true}
	runbook := agentapi.AutonomyRunbook(base)
	if got := runbook["trigger_contract"]; got != "operator_schedule_channel" {
		t.Errorf("a direct-callable agent triggers through the operator schedule channel, got %v", got)
	}
	if got := runbook["route_contract"]; got != "self_owned" {
		t.Errorf("a direct-callable agent routes to itself, got %v", got)
	}
	if got := runbook["sleep_contract"]; got != agentapi.LifecyclePersistent {
		t.Errorf("an agent with no lifecycle sleeps persistent, got %v", got)
	}
	if got := runbook["recovery_contract"]; got != "manual" {
		t.Errorf("an agent with no policy recovers manually, got %v", got)
	}
	if got := runbook["retry_attempts"]; got != 1 {
		t.Errorf("an agent with no retry policy gets one attempt, got %v", got)
	}

	// A managed (not direct-callable) agent is delegation-only and routes to
	// its manager — the parent wins over the owner.
	no := false
	child := agentapi.Profile{Slug: "worker", Enabled: true, DirectCallable: &no, ParentAgent: "lead", OwnerAgent: "owner"}
	runbook = agentapi.AutonomyRunbook(child)
	if got := runbook["trigger_contract"]; got != "delegation_only" {
		t.Errorf("a managed agent is delegation-only, got %v", got)
	}
	if got := runbook["route_contract"]; got != "leader:lead" {
		t.Errorf("a managed agent routes to its parent, got %v", got)
	}
	if got := runbook["delegation_manager"]; got != "lead" {
		t.Errorf("the delegation manager is the parent, got %v", got)
	}

	// A managed agent with NO manager cannot be routed: the contract says so
	// rather than pretending it is reachable.
	orphan := agentapi.Profile{Slug: "orphan", Enabled: true, DirectCallable: &no}
	runbook = agentapi.AutonomyRunbook(orphan)
	if got := runbook["route_contract"]; got != "leader_missing" {
		t.Errorf("a manager-less agent must report a missing leader, got %v", got)
	}

	// A retired agent is blocked, whatever else it says.
	retired := agentapi.Profile{Slug: "old", Enabled: true, Retired: true}
	runbook = agentapi.AutonomyRunbook(retired)
	if got := runbook["trigger_contract"]; got != "blocked_retired" {
		t.Errorf("a retired agent is blocked, got %v", got)
	}
	if got := runbook["sleep_contract"]; got != "graveyard" {
		t.Errorf("a retired agent sleeps in the graveyard, got %v", got)
	}

	// A paused agent is blocked too — but it is not a graveyard agent.
	paused := agentapi.Profile{Slug: "nap", Enabled: false}
	runbook = agentapi.AutonomyRunbook(paused)
	if got := runbook["trigger_contract"]; got != "blocked_paused" {
		t.Errorf("a paused agent is blocked, got %v", got)
	}
	if got := runbook["sleep_contract"]; got != "paused" {
		t.Errorf("a paused agent sleeps paused, got %v", got)
	}

	// The recovery contract reads the policies in order: self-repair, then the
	// doctor, then the retry budget.
	repairing := agentapi.Profile{Slug: "healer", Enabled: true}
	repairing.SelfRepairPolicy = &agentapi.SelfRepairPolicy{Enabled: true, MaxAttempts: 3}
	repairing.HealthPolicy = &agentapi.HealthPolicy{DoctorAgent: "doctor"}
	repairing.RetryPolicy = &agentapi.RetryPolicy{MaxAttempts: 5}
	runbook = agentapi.AutonomyRunbook(repairing)
	if got := runbook["recovery_contract"]; got != "self_repair" {
		t.Errorf("a self-repairing agent recovers by self-repair, got %v", got)
	}
	if got := runbook["self_repair_enabled"]; got != true {
		t.Errorf("the runbook must report the self-repair flag, got %v", got)
	}
	if got := runbook["self_repair_attempts"]; got != 3 {
		t.Errorf("the runbook must report the self-repair budget, got %v", got)
	}

	doctored := agentapi.Profile{Slug: "watched", Enabled: true}
	doctored.HealthPolicy = &agentapi.HealthPolicy{DoctorAgent: "doctor"}
	runbook = agentapi.AutonomyRunbook(doctored)
	if got := runbook["recovery_contract"]; got != "doctor:doctor" {
		t.Errorf("a doctored agent recovers through its doctor, got %v", got)
	}
	if got := runbook["doctor_agent"]; got != "doctor" {
		t.Errorf("the runbook must name the doctor, got %v", got)
	}

	retried := agentapi.Profile{Slug: "flaky", Enabled: true}
	retried.RetryPolicy = &agentapi.RetryPolicy{MaxAttempts: 4}
	runbook = agentapi.AutonomyRunbook(retried)
	if got := runbook["recovery_contract"]; got != "retry" {
		t.Errorf("a retrying agent recovers by retry, got %v", got)
	}
	if got := runbook["retry_attempts"]; got != 4 {
		t.Errorf("the retry budget is %v, want 4", got)
	}

	// The sleep contract reads the lifecycle: an explicit mode wins, then the
	// retire-on-complete flag, then the cycle budget.
	cycling := agentapi.Profile{Slug: "cycler", Enabled: true}
	cycling.Lifecycle = agentapi.AgentLifecycle{MaxCycles: 3}
	runbook = agentapi.AutonomyRunbook(cycling)
	if got := runbook["sleep_contract"]; got != agentapi.LifecycleCycle {
		t.Errorf("an agent with a cycle budget sleeps cycle, got %v", got)
	}
	retiring := agentapi.Profile{Slug: "oneshot", Enabled: true}
	retiring.Lifecycle = agentapi.AgentLifecycle{RetireOnComplete: true}
	runbook = agentapi.AutonomyRunbook(retiring)
	if got := runbook["sleep_contract"]; got != agentapi.LifecycleRetireOnComplete {
		t.Errorf("a retire-on-complete agent sleeps retire_on_complete, got %v", got)
	}
}
