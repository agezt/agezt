// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/roster"
)

func TestScheduleCoverageBindAndCurrent(t *testing.T) {
	tool := New()
	if tool.store != nil {
		t.Fatal("new tool should have nil store")
	}
	// Bind(nil) is a no-op.
	tool.Bind(nil)
	if tool.store != nil {
		t.Fatal("Bind(nil) should not set store")
	}

	// current() with defaults.
	st, now, lookup := tool.current()
	if st != nil {
		t.Fatalf("current store = %p, want nil", st)
	}
	if now == nil {
		t.Fatal("current now should be non-nil")
	}
	if lookup != nil {
		t.Fatal("current lookup should be nil")
	}

	// BindAgentLookup wires the lookup function.
	called := false
	tool.BindAgentLookup(func(string) (roster.Profile, bool) { called = true; return roster.Profile{}, false })
	if !called {
		// touch it
		lookupNow := tool.agentLookup
		if lookupNow != nil {
			lookupNow("anything")
		}
	}
	if tool.agentLookup == nil {
		t.Fatal("agentLookup should be set")
	}
}

func TestScheduleCoverageScheduleBindsActingAgent(t *testing.T) {
	cases := []struct {
		in   input
		want bool
	}{
		{in: input{}, want: true},
		{in: input{Workflow: "wf"}, want: false},
		{in: input{System: "system_task"}, want: false},
		{in: input{Tool: "tool_name"}, want: false},
		// "agent" is aliased to the intent target; "intent" is the same.
		{in: input{Target: "agent"}, want: true},
		{in: input{Target: ""}, want: true},
		{in: input{Target: "workflow"}, want: true},
		{in: input{Target: "tool"}, want: true},
		{in: input{Target: "system_task"}, want: false},
	}
	for _, tc := range cases {
		if got := scheduleBindsActingAgent(tc.in); got != tc.want {
			t.Fatalf("scheduleBindsActingAgent(%+v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestScheduleCoverageScheduleTarget(t *testing.T) {
	cases := []struct {
		in   input
		want string
	}{
		{in: input{}, want: ""},
		{in: input{Workflow: "wf"}, want: "workflow"},
		{in: input{System: "reaper"}, want: "system_task"},
		{in: input{Tool: "fetch"}, want: "tool"},
		// "agent" is aliased to the intent target (TargetIntent is "").
		{in: input{Target: "agent"}, want: ""},
		{in: input{Target: ""}, want: ""},
		{in: input{Target: "workflow"}, want: "workflow"},
		{in: input{Target: "system_task"}, want: "system_task"},
		{in: input{Target: "tool"}, want: "tool"},
		// "agent" is unconditionally aliased to the intent target.
		{in: input{Target: "agent", Workflow: "wf"}, want: ""},
		// When only typed fields are set without Target, the typed field wins.
		{in: input{Workflow: "wf"}, want: "workflow"},
		// When Target and typed field are set, Target's alias rules apply first
		// (so "agent" → ""), but the workflow branch has its own checks.
		{in: input{Target: "workflow", Workflow: "wf"}, want: "workflow"},
	}
	for _, tc := range cases {
		if got := scheduleTarget(tc.in); got != tc.want {
			t.Fatalf("scheduleTarget(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestScheduleCoverageValidateActingAgent(t *testing.T) {
	// No lookup + acting-agent schedule → no error (early return).
	res := validateActingAgentSchedule(context.Background(), input{}, nil)
	if res.Output != "" {
		t.Fatalf("nil lookup = %+v", res)
	}

	// Acting agent (intent target) with no slug in context → no error.
	res = validateActingAgentSchedule(context.Background(), input{Target: "agent"}, func(string) (roster.Profile, bool) { return roster.Profile{}, false })
	if res.Output != "" {
		t.Fatalf("no agent slug = %+v", res)
	}

	// Slug in context but unknown to lookup → "not in the roster".
	ctx := agent.WithAgent(context.Background(), "missing")
	res = validateActingAgentSchedule(ctx, input{Target: "agent"}, func(slug string) (roster.Profile, bool) {
		return roster.Profile{}, false
	})
	if !res.IsError || !strings.Contains(res.Output, "not in the roster") {
		t.Fatalf("unknown agent = %+v", res)
	}

	// Retired agent → "is retired".
	res = validateActingAgentSchedule(ctx, input{Target: "agent"}, func(slug string) (roster.Profile, bool) {
		return roster.Profile{Slug: "a", Retired: true}, true
	})
	if !res.IsError || !strings.Contains(res.Output, "retired") {
		t.Fatalf("retired agent = %+v", res)
	}

	// Disabled agent → "is paused".
	res = validateActingAgentSchedule(ctx, input{Target: "agent"}, func(slug string) (roster.Profile, bool) {
		return roster.Profile{Slug: "a", Enabled: false}, true
	})
	if !res.IsError || !strings.Contains(res.Output, "paused") {
		t.Fatalf("disabled agent = %+v", res)
	}
}

func TestScheduleCoverageManagedSubAgentHint(t *testing.T) {
	cases := []struct {
		p    roster.Profile
		want []string
	}{
		{p: roster.Profile{Slug: "x", ParentAgent: "boss"}, want: []string{"boss", "managed sub-agent"}},
		{p: roster.Profile{Slug: "x", OwnerAgent: "owner"}, want: []string{"owner", "managed sub-agent"}},
		{p: roster.Profile{Slug: "x"}, want: []string{"managed sub-agent", "route the work"}},
	}
	for _, tc := range cases {
		got := managedSubAgentScheduleHint(tc.p)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("hint = %q, want substring %q", got, want)
			}
		}
	}
}

func TestScheduleCoverageInvokeValidation(t *testing.T) {
	tool := New()
	// No store bound.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unavailable = %+v", res)
	}

	// Parse error → hard error.
	_, err = New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}
}
