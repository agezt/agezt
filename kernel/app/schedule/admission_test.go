// SPDX-License-Identifier: MIT

package schedule_test

import (
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"reflect"
	"testing"
)

func TestScheduleAdmissionRetainsLiveLookupOrderAndExactErrors(t *testing.T) {
	no := false
	for _, mode := range []string{"intent", "missing-agent", "retired", "paused", "managed", "empty-workflow", "missing-workflow", "workflow", "system-task", "missing-system-task", "empty-tool", "missing-tool", "tool", "denied-tool", "agent-disappears", "unknown-target"} {
		t.Run(mode, func(t *testing.T) {
			entry := cadence.Entry{Target: cadence.TargetIntent, Agent: " raw-agent "}
			profile := roster.Profile{Slug: "canonical", Enabled: true}
			found := true
			want := ""
			calls := []string{}
			expected := []string{"agent: raw-agent "}
			agentCalls := 0
			switch mode {
			case "missing-agent":
				found = false
				want = "unknown agent:  raw-agent "
			case "retired":
				profile.Retired = true
				profile.Enabled = false
				want = "agent canonical is retired — revive it first"
			case "paused":
				profile.Enabled = false
				want = "agent canonical is paused"
			case "managed":
				profile.DirectCallable = &no
				want = "selected managed error"
				expected = append(expected, "managed:canonical:scheduled")
			case "empty-workflow":
				entry.Target = cadence.TargetWorkflow
				entry.Workflow = " "
				want = "workflow schedule missing workflow target"
			case "missing-workflow", "workflow":
				entry.Target = cadence.TargetWorkflow
				entry.Workflow = " raw-flow "
				expected = append(expected, "workflow: raw-flow ")
				if mode == "missing-workflow" {
					want = "unknown workflow:  raw-flow "
				}
			case "system-task":
				entry.Target = cadence.TargetSystemTask
				entry.SystemTask = cadence.SystemTaskCatalogSync
			case "missing-system-task":
				entry.Target = cadence.TargetSystemTask
				entry.SystemTask = "unknown"
				want = "unknown system task: unknown"
			case "empty-tool":
				entry.Target = cadence.TargetTool
				entry.Tool = " "
				want = "tool schedule missing tool target"
			case "missing-tool", "tool", "denied-tool", "agent-disappears":
				entry.Target = cadence.TargetTool
				entry.Tool = " raw-tool "
				expected = append(expected, "tool: raw-tool ")
				if mode == "missing-tool" {
					want = "unknown tool:  raw-tool "
				} else {
					expected = append(expected, "agent: raw-agent ")
				}
				if mode == "denied-tool" {
					profile.ToolDeny = []string{" RAW-TOOL "}
					want = "agent canonical cannot schedule tool  raw-tool : agent tool denylist"
				}
				if mode == "agent-disappears" {
					want = "unknown agent:  raw-agent "
				}
			case "unknown-target":
				entry.Target = "bad"
				want = "unknown schedule target: bad"
			}
			service := appschedule.NewAdmission(appschedule.AdmissionHost{
				Agent: func(ref string) (roster.Profile, bool) {
					calls = append(calls, "agent:"+ref)
					agentCalls++
					return profile, found && (mode != "agent-disappears" || agentCalls == 1)
				},
				Workflow: func(ref string) bool { calls = append(calls, "workflow:"+ref); return mode == "workflow" },
				Tool:     func(ref string) bool { calls = append(calls, "tool:"+ref); return mode != "missing-tool" },
				ManagedDirectError: func(p roster.Profile, action string) string {
					calls = append(calls, "managed:"+p.Slug+":"+action)
					return "selected managed error"
				},
			})
			err := service.Runnable(entry)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != want || !reflect.DeepEqual(calls, expected) {
				t.Fatal(got, want, calls, expected)
			}
		})
	}
}
func TestScheduleAgentToolAdmissionRetainsNormalizationDenyPrecedenceAndDefaultAllow(t *testing.T) {
	for _, tc := range []struct {
		name        string
		allow, deny []string
		tool, want  string
	}{
		{name: "default", tool: "unknown"}, {name: "empty", allow: []string{"read"}, deny: []string{" "}, tool: " "},
		{name: "normalized", allow: []string{"", " READ "}, tool: " read "},
		{name: "denied", allow: []string{"read"}, deny: []string{" READ "}, tool: "read", want: "agent canonical cannot schedule tool read: agent tool denylist"},
		{name: "allow refusal", allow: []string{"read"}, tool: "write", want: "agent canonical cannot schedule tool write: not in agent tool allowlist"},
		{name: "empty allow", allow: []string{" "}, tool: "write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := appschedule.ValidateAgentTool(roster.Profile{Slug: "canonical", ToolAllow: tc.allow, ToolDeny: tc.deny}, tc.tool)
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}
func TestScheduleFrequencyWarningRetainsModeThresholdAndPrecedence(t *testing.T) {
	const task = "system task runs more frequently than its recommended cadence"
	const guardian = "system agent schedule is more frequent than the guardian quiet window"
	const wake = "agent wake schedule is very frequent"
	service := appschedule.NewAdmission(appschedule.AdmissionHost{Agent: func(ref string) (roster.Profile, bool) {
		return roster.Profile{System: ref == "guardian"}, ref != "missing"
	}})
	cases := []struct {
		entry cadence.Entry
		want  string
	}{
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 0}, ""},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 899}, wake},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 900}, ""},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 60, Mode: cadence.ModeOnce}, ""},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 60, Mode: cadence.ModeDaily}, ""},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 60, Mode: cadence.ModeWindow}, wake},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 60, Mode: cadence.ModeContinuous}, wake},
		{cadence.Entry{Target: cadence.TargetIntent, IntervalSec: 60, Agent: "guardian"}, guardian},
		{cadence.Entry{Target: cadence.TargetTool, IntervalSec: 28799, Agent: "guardian"}, guardian},
		{cadence.Entry{Target: cadence.TargetTool, IntervalSec: 28800, Agent: "guardian"}, ""},
		{cadence.Entry{Target: cadence.TargetTool, IntervalSec: 60, Agent: "missing"}, ""},
		{cadence.Entry{Target: cadence.TargetSystemTask, SystemTask: cadence.SystemTaskCatalogSync, IntervalSec: 86399, Agent: "guardian"}, task},
		{cadence.Entry{Target: cadence.TargetSystemTask, SystemTask: cadence.SystemTaskCatalogSync, IntervalSec: 86400, Agent: "guardian"}, ""},
		{cadence.Entry{Target: cadence.TargetSystemTask, SystemTask: "unknown", IntervalSec: 60, Agent: "guardian"}, ""},
	}
	for _, tc := range cases {
		if got := service.Warning(tc.entry); got != tc.want {
			t.Fatal(tc.entry, got, tc.want)
		}
	}
}
