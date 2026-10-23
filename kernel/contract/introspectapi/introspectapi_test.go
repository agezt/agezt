// SPDX-License-Identifier: MIT

package introspectapi

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/agezt/agezt/kernel/contract/types"
)

// TestDelegationIsTheContractType: the delegation row IS the contract type,
// not a copy of it. The kernel's ceilings therefore reach the agent without a
// field-by-field projection.
func TestDelegationIsTheContractType(t *testing.T) {
	if reflect.TypeOf(Delegation{}) != reflect.TypeOf(types.SubAgentLimits{}) {
		t.Fatalf("the delegation row drifted from the contract type: %v vs %v",
			reflect.TypeOf(Delegation{}), reflect.TypeOf(types.SubAgentLimits{}))
	}
}

// wireKeys returns the JSON object keys of the marshalled value, sorted.
func wireKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(wire))
	for k := range wire {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestOverviewWireShape: the overview's wire is the daemon's public health
// report — the agent reads it, so the key set is part of the contract.
func TestOverviewWireShape(t *testing.T) {
	got := wireKeys(t, Overview{})
	want := []string{
		"active_runs", "active_skills", "daemon", "delegation", "halted",
		"journal_head", "memory_records", "model", "pending_approvals",
		"protocol", "provider_fallbacks", "reaper", "schedules_enabled",
		"schedules_total", "tools", "uptime_seconds", "world_entities",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the overview wire keys = %v, want %v", got, want)
	}
}

// TestOverviewAlwaysFieldsAreAlwaysPresent: the fields that describe the
// daemon's state are always on the wire — a missing "halted" would read as
// "false" and hide a halted daemon from the agent.
func TestOverviewAlwaysFieldsAreAlwaysPresent(t *testing.T) {
	raw, err := json.Marshal(Overview{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"daemon", "protocol", "model", "uptime_seconds", "halted", "active_runs",
		"tools", "memory_records", "world_entities", "active_skills", "journal_head",
		"schedules_total", "schedules_enabled", "pending_approvals",
		"provider_fallbacks", "delegation", "reaper",
	} {
		if _, ok := wire[k]; !ok {
			t.Errorf("the overview wire is missing the always-field %q: %s", k, raw)
		}
	}
	// The one optional field: a daemon with no fallback has no reason to
	// publish, so it stays off the wire.
	if _, ok := wire["provider_fallback_reason"]; ok {
		t.Errorf("an empty fallback reason must stay off the wire: %s", raw)
	}
}

// TestReaperReportWireShape: the reaper's report always carries the two
// numbers (how much is stale) and only carries the agent lists when there
// are agents in that bucket.
func TestReaperReportWireShape(t *testing.T) {
	got := wireKeys(t, ReaperReport{})
	want := []string{"stale_artifacts", "stale_bytes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the reaper report wire keys = %v, want %v", got, want)
	}

	raw, err := json.Marshal(ReaperReport{
		DeadAgents:          []ReaperAgent{{Slug: "a"}},
		StaleArtifacts:      3,
		StaleBytes:          42,
		DegradedAgents:      []ReaperDegradedAgent{{Slug: "b"}},
		MisconfiguredAgents: []ReaperMisconfiguredAgent{{Slug: "c"}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wire["stale_artifacts"] != float64(3) || wire["stale_bytes"] != float64(42) {
		t.Fatalf("the reaper report lost the stale counters: %s", raw)
	}
	for _, k := range []string{"dead_agents", "degraded_agents", "misconfigured_agents"} {
		list, ok := wire[k].([]any)
		if !ok || len(list) != 1 {
			t.Fatalf("the reaper report lost the %q bucket: %s", k, raw)
		}
	}
}

// TestReaperOverviewWireShape: the fleet snapshot inside the overview always
// carries the three counts; the slug lists are omitted when the bucket is
// empty.
func TestReaperOverviewWireShape(t *testing.T) {
	got := wireKeys(t, ReaperOverview{})
	want := []string{"dead_agents", "degraded_agents", "misconfigured_agents"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the reaper overview wire keys = %v, want %v", got, want)
	}

	raw, err := json.Marshal(ReaperOverview{DeadAgents: 1, DeadSlugs: []string{"a"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wire["dead_agents"] != float64(1) {
		t.Fatalf("the reaper overview lost the dead count: %s", raw)
	}
	if _, ok := wire["dead_slugs"]; !ok {
		t.Fatalf("the reaper overview lost the dead slugs: %s", raw)
	}
	if _, ok := wire["degraded_slugs"]; ok {
		t.Fatalf("an empty slug bucket must stay off the wire: %s", raw)
	}
}

// TestTheReaperAgentRowsWireShape: the three agent rows inside the reaper's
// report are part of the wire too — the agent reads the counts and the rows,
// so the key sets are pinned.
func TestTheReaperAgentRowsWireShape(t *testing.T) {
	for _, c := range []struct {
		name string
		row  any
		want []string
	}{
		{"ReaperAgent", ReaperAgent{}, []string{"slug"}},
		{"ReaperDegradedAgent", ReaperDegradedAgent{}, []string{"slug"}},
		{"ReaperMisconfiguredAgent", ReaperMisconfiguredAgent{}, []string{"slug"}},
	} {
		got := wireKeys(t, c.row)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("the %s wire keys = %v, want %v", c.name, got, c.want)
		}
	}

	// The always-present counters of a degraded agent: the agent reads them to
	// judge whether the degradation is worth escalating.
	raw, err := json.Marshal(ReaperDegradedAgent{
		Slug: "a", Failures: 3, Window: 10, Threshold: 2,
		DoctorAgent: "doctor", SelfRepairEnabled: true, EscalateTo: "human",
		LastFailureMS: 99, LastReason: "boom",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"slug", "failures", "window", "threshold", "doctor_agent",
		"self_repair_enabled", "escalate_to", "last_failure_ms", "last_reason",
	} {
		if _, ok := wire[k]; !ok {
			t.Errorf("the degraded agent row lost %q: %s", k, raw)
		}
	}
}

// TestSourcePortShape: the port is the tool's whole live-state surface —
// four reads, no more. A port that grows a method grows the plugin's reach.
func TestSourcePortShape(t *testing.T) {
	pt := reflect.TypeOf((*Source)(nil)).Elem()
	if pt.Kind() != reflect.Interface {
		t.Fatalf("Source must be an interface, got %v", pt.Kind())
	}
	want := []string{"Overview", "Reaper", "Schedules", "Standing"}
	got := make([]string, 0, pt.NumMethod())
	for i := 0; i < pt.NumMethod(); i++ {
		got = append(got, pt.Method(i).Name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the source port methods = %v, want %v", got, want)
	}
}
