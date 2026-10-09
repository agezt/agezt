// SPDX-License-Identifier: MIT

package reaper

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
)

var clock = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func scanRequest(t *testing.T, raw string) ScanRequest {
	t.Helper()
	var in ScanRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestScanDaysAndCutoffs(t *testing.T) {
	for raw, want := range map[string][2]int{
		`{}`:                                  {30, 30},
		`{"idle_days":7,"stale_days":2}`:      {7, 2},
		`{"idle_days":1.9,"stale_days":0.5}`:  {1, 30},
		`{"idle_days":0,"stale_days":-3}`:     {30, 30},
		`{"idle_days":"7","stale_days":null}`: {30, 30},
		`{"idle_days":true,"stale_days":[1]}`: {30, 30},
	} {
		var cuts []int64
		svc := New(func(a, b int64) runtime.ReaperReport { cuts = []int64{a, b}; return runtime.ReaperReport{} }, func() time.Time { return clock })
		out, err := svc.Scan(context.Background(), scanRequest(t, raw))
		if err != nil || out.IdleDays != want[0] || out.StaleDays != want[1] {
			t.Fatal(raw, out.IdleDays, out.StaleDays, err)
		}
		wantCuts := []int64{clock.Add(-time.Duration(want[0]) * 24 * time.Hour).UnixMilli(), clock.Add(-time.Duration(want[1]) * 24 * time.Hour).UnixMilli()}
		if !reflect.DeepEqual(cuts, wantCuts) {
			t.Fatal("both cutoffs count back from the daemon clock", raw, cuts, wantCuts)
		}
	}
}

func TestScanRows(t *testing.T) {
	rep := runtime.ReaperReport{
		DeadAgents:             []runtime.ReaperAgent{{Slug: "d", LastActiveMS: 0}},
		DegradedAgents:         []runtime.DegradedAgent{{Slug: "g", Name: "G", Failures: 2, Window: 3, Threshold: 2, DoctorAgent: "doc", SelfRepairEnabled: true, EscalateTo: "lead", LastFailureMS: 5, LastReason: "error"}},
		MisconfiguredAgents:    []runtime.MisconfiguredAgent{{Slug: "m"}, {Slug: "m2", Issues: []string{"a", "b"}}},
		RetryPressure:          []runtime.RetryPressureAgent{{Slug: "r", Count: 3, Threshold: 3, WindowSec: 60, LastRetryMS: 9, NextAttempt: 2, MaxAttempts: 3}},
		RoutingPressure:        []runtime.RoutingPressureAgent{{Slug: "p", LastFailedModel: "m1", LastNextModel: "m2", TaskType: "code"}},
		RoutingForced:          []runtime.RoutingForcedProbationAgent{{Slug: "fp", ForcedChain: []string{"x"}, ForceGeneration: 1, LastForcedMS: 4}},
		RoutingForcedFailed:    []runtime.RoutingForcedFailedAgent{{Slug: "ff", ForcedChain: []string{"y"}, ForceGeneration: 2}},
		RoutingForcedExhausted: []runtime.RoutingForcedExhaustedAgent{{Slug: "fe", ForceGeneration: 3}},
		RoutingUnstable:        []runtime.RoutingUnstableAgent{{Slug: "u", CurrentChain: []string{"c"}, PreviousChain: []string{"p"}, LastRollbackMS: 7}},
		StaleArtifacts:         4,
		StaleBytes:             9007199254740993,
	}
	out, err := New(func(int64, int64) runtime.ReaperReport { return rep }, func() time.Time { return clock }).Scan(context.Background(), ScanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if out.DeadCount != 1 || out.DegradedCount != 1 || out.MisconfiguredCount != 2 || out.RetryPressureCount != 1 || out.RoutingPressureCount != 1 || out.ForcedProbationCount != 1 || out.ForcedFailedCount != 1 || out.ForcedExhaustedCount != 1 || out.RoutingUnstableCount != 1 || out.StaleArtifacts != 4 || out.StaleBytes != 9007199254740993 {
		t.Fatalf("each list carries its count: %+v", out)
	}
	if !reflect.DeepEqual(out.DegradedAgents[0], DegradedRow{Slug: "g", Name: "G", Failures: 2, Window: 3, Threshold: 2, DoctorAgent: "doc", SelfRepairEnabled: true, EscalateTo: "lead", LastFailureMS: 5, LastReason: "error"}) ||
		!reflect.DeepEqual(out.RetryPressureAgents[0], RetryRow{Slug: "r", Count: 3, Threshold: 3, WindowSec: 60, LastRetryMS: 9, NextAttempt: 2, MaxAttempts: 3}) ||
		!reflect.DeepEqual(out.RoutingPressureAgents[0], RoutingRow{Slug: "p", LastFailedModel: "m1", LastNextModel: "m2", TaskType: "code"}) ||
		!reflect.DeepEqual(out.ForcedProbationAgents[0], ForcedRow{Slug: "fp", ForcedChain: []string{"x"}, ForceGeneration: 1, LastForcedMS: 4}) ||
		!reflect.DeepEqual(out.ForcedFailedAgents[0], ForcedRow{Slug: "ff", ForcedChain: []string{"y"}, ForceGeneration: 2}) ||
		!reflect.DeepEqual(out.ForcedExhaustedAgents[0], ForcedRow{Slug: "fe", ForceGeneration: 3}) ||
		!reflect.DeepEqual(out.RoutingUnstableAgents[0], UnstableRow{Slug: "u", CurrentChain: []string{"c"}, PreviousChain: []string{"p"}, LastRollbackMS: 7}) ||
		!reflect.DeepEqual(out.MisconfiguredAgents[1].Issues, []string{"a", "b"}) || out.DeadAgents[0] != (DeadRow{Slug: "d"}) {
		t.Fatalf("rows carry every field: %+v", out)
	}
	raw, _ := json.Marshal(out)
	for _, want := range []string{`"dead_agents":[{"slug":"d","name":"","last_active_ms":0}]`, `"issues":null`, `"forced_chain":null`, `"stale_bytes":9007199254740993`, `"routing_force_generation":3`} {
		if !strings.Contains(string(raw), want) {
			t.Fatal("rows keep zero fields and unset lists stay null", want, string(raw))
		}
	}
	empty, _ := New(func(int64, int64) runtime.ReaperReport { return runtime.ReaperReport{} }, time.Now).Scan(context.Background(), ScanRequest{})
	raw, _ = json.Marshal(empty)
	for _, key := range []string{"dead_agents", "degraded_agents", "misconfigured_agents", "retry_pressure_agents", "routing_pressure_agents", "routing_forced_probation_agents", "routing_forced_failed_agents", "routing_forced_exhausted_agents", "routing_unstable_agents"} {
		if !strings.Contains(string(raw), `"`+key+`":[]`) {
			t.Fatal("an empty finding is an empty list", key, string(raw))
		}
	}
}

func TestOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	svc := New(func(int64, int64) runtime.ReaperReport {
		return runtime.ReaperReport{MisconfiguredAgents: []runtime.MisconfiguredAgent{{Slug: "m"}}}
	}, time.Now)
	ops, err := Operations(func(context.Context) *Service { return svc })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "reaper_scan" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{Method: "GET", Path: "/api/reaper/scan"}) {
		t.Fatal(spec)
	}
	out, _ := svc.Scan(context.Background(), ScanRequest{})
	raw, _ := json.Marshal(out)
	if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
		t.Fatal(err, string(raw))
	}
}
