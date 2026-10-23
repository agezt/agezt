// SPDX-License-Identifier: MIT

package roster

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// The roster's rows and the contract's rows must be the SAME types.
//
// The host's methods speak the roster's names (kernel/runtime and the control
// plane alias them), the tools that steer the roster speak the contract's
// names (the overseer tool is a plugin, and a plugin may only reach the
// contract), and the daemon satisfies both. That wiring only compiles when
// the two are ONE type — so if the roster ever declares its own copy of a row
// instead of aliasing the contract's, the break shows up at the far end of
// the tree.
//
// The function-type form is deliberate: an ASSIGNMENT would also accept a
// copy with the same fields, but a function type requires IDENTITY. So these
// assertions say so at the seam, not at the far end.
//
// They are compile-time facts, so the "test" is that this file builds.
var (
	_ func(agentapi.Profile) string       = func(p agentapi.Profile) string { return p.Kind() }
	_ func(agentapi.Profile) bool         = func(p agentapi.Profile) bool { return p.AllowsDirectCall() }
	_ func(agentapi.Profile, string) bool = func(p agentapi.Profile, caller string) bool {
		return p.AllowsDelegationFrom(caller)
	}
)

// TestTheRowsAreTheContractRows states the same fact at runtime: the roster's
// row and the contract's row are interchangeable, field for field, and a
// change to one that does not reach the other breaks here.
func TestTheRowsAreTheContractRows(t *testing.T) {
	p := Profile{
		ID:              "id-1",
		Slug:            "researcher",
		Name:            "Researcher",
		Soul:            "you research",
		Model:           "m1",
		Fallbacks:       []string{"m2"},
		TaskType:        "research",
		MaxCostMc:       10,
		MaxDailyMc:      20,
		MemoryScope:     "scope",
		Workdir:         "wd",
		OwnerAgent:      "lead",
		ParentAgent:     "lead",
		TrustCeiling:    "L4",
		ConfigOverrides: map[string]string{"AGEZT_X": "1"},
		Lifecycle:       AgentLifecycle{Mode: LifecycleCycle, MaxCycles: 3},
		TaskList:        []AgentTask{{ID: "t1", Title: "do"}},
		Enabled:         true,
		CreatedMS:       11,
		UpdatedMS:       12,
	}
	var contract agentapi.Profile = p
	if contract.Slug != p.Slug || contract.Soul != p.Soul || contract.Model != p.Model {
		t.Fatalf("the profile did not survive the crossing: %+v", contract)
	}
	if len(contract.Fallbacks) != 1 || contract.Fallbacks[0] != "m2" {
		t.Fatalf("the fallbacks did not survive the crossing: %+v", contract.Fallbacks)
	}
	if contract.Lifecycle.Mode != LifecycleCycle || contract.Lifecycle.MaxCycles != 3 {
		t.Fatalf("the lifecycle did not survive the crossing: %+v", contract.Lifecycle)
	}
	if len(contract.TaskList) != 1 || contract.TaskList[0].Title != "do" {
		t.Fatalf("the task list did not survive the crossing: %+v", contract.TaskList)
	}
	if contract.ConfigOverrides["AGEZT_X"] != "1" {
		t.Fatalf("the config overrides did not survive the crossing: %+v", contract.ConfigOverrides)
	}
}

// TestProfileWireShape pins the row's wire keys. The Web UI, the SDK and the
// persisted roster files all read the profile by KEY NAME, so a renamed key is
// a silent break for every consumer that does not live in this package — and
// no test in the package would notice, because the package writes and reads
// the same struct.
func TestProfileWireShape(t *testing.T) {
	yes := true
	full := Profile{
		ID:               "id-1",
		Slug:             "researcher",
		Name:             "Researcher",
		Soul:             "you research",
		Instructions:     []string{"be careful"},
		Model:            "m1",
		Fallbacks:        []string{"m2"},
		TaskType:         "research",
		MaxCostMc:        10,
		MaxDailyMc:       20,
		MemoryScope:      "scope",
		Workdir:          "wd",
		OwnerAgent:       "lead",
		ParentAgent:      "lead",
		DirectCallable:   &yes,
		RetryPolicy:      &RetryPolicy{MaxAttempts: 2},
		HealthPolicy:     &HealthPolicy{DoctorAgent: "doctor"},
		SelfRepairPolicy: &SelfRepairPolicy{Enabled: true},
		NoisePolicy:      &NoisePolicy{SilentOnSuccess: true},
		ToolAllow:        []string{"shell"},
		ToolDeny:         []string{"rm"},
		TrustCeiling:     "L4",
		ExecutionProfile: "local",
		ConfigOverrides:  map[string]string{"AGEZT_X": "1"},
		Lifecycle:        AgentLifecycle{Mode: LifecycleCycle},
		TaskList:         []AgentTask{{ID: "t1"}},
		Description:      "desc",
		Enabled:          true,
		Retired:          true,
		RetiredMS:        5,
		RetiredReason:    "done",
		System:           true,
		CreatedMS:        11,
		UpdatedMS:        12,
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{
		"id", "slug", "name", "soul", "instructions", "model", "fallbacks",
		"task_type", "max_cost_mc", "max_daily_mc", "memory_scope", "workdir",
		"owner_agent", "parent_agent", "direct_callable", "retry_policy",
		"health_policy", "self_repair", "noise_policy", "tool_allow", "tool_deny",
		"trust_ceiling", "execution_profile", "config_overrides", "lifecycle",
		"tasklist", "description", "enabled", "retired", "retired_ms",
		"retired_reason", "system", "created_ms", "updated_ms",
	} {
		if _, ok := out[key]; !ok {
			t.Errorf("the profile's wire is missing %q", key)
		}
	}
	// ...and no camelCase twin crept in beside it.
	if _, ok := out["taskType"]; ok {
		t.Error("the profile's wire grew a camelCase key beside task_type")
	}
}

// TestAllowsDirectCall pins the direct-call policy in all three states: an
// explicit true, an explicit false, and the unset default (old profiles stay
// callable). The "explicit true" case is the one the mutation run would slip
// through — the other two are covered indirectly by the Kind test.
func TestAllowsDirectCall(t *testing.T) {
	yes, no := true, false
	if !(Profile{Slug: "a"}).AllowsDirectCall() {
		t.Error("an unset direct-callable flag must default to callable")
	}
	if !(Profile{Slug: "a", DirectCallable: &yes}).AllowsDirectCall() {
		t.Error("an explicitly callable agent must be callable")
	}
	if (Profile{Slug: "a", DirectCallable: &no}).AllowsDirectCall() {
		t.Error("an explicitly managed agent must not be directly callable")
	}
}
