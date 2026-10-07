// SPDX-License-Identifier: MIT

package schedule_test

import (
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"reflect"
	"testing"
)

func TestScheduleEditTargetRetainsPresenceInheritanceAndNonCanonicalWorkflowPreflight(t *testing.T) {
	for _, mode := range []string{"absent", "explicit-intent", "agent-clear", "agent-alias", "workflow-alias", "tool-inherited", "system-inherited", "tool-payload-ignored"} {
		t.Run(mode, func(t *testing.T) {
			in := appschedule.EditTargetInput{Current: cadence.Entry{Target: cadence.TargetWorkflow, Agent: " inherited "}, Payload: make(chan int), PayloadPresent: true}
			want := appschedule.EditTarget{}
			calls := []string{}
			expected := []string{}
			profile := roster.Profile{Slug: "canonical", Enabled: true}
			switch mode {
			case "explicit-intent":
				in.Current.Target = cadence.TargetSystemTask
				in.TargetPresent = true
			case "agent-clear":
				in.Current.Target = cadence.TargetTool
				in.AgentPresent = true
			case "agent-alias":
				in.Agent = " alias "
				in.AgentPresent = true
				want.Agent = "canonical"
				expected = []string{"agent:alias"}
			case "workflow-alias":
				in.Workflow = " alias-flow "
				in.Payload = nil
				want.Target = cadence.TargetWorkflow
				want.Workflow = "alias-flow"
				expected = []string{"workflow:alias-flow"}
			case "tool-inherited":
				in.Tool = " shell "
				in.Payload = nil
				profile.Enabled = false
				profile.Retired = true
				want.Target = cadence.TargetTool
				want.Tool = "shell"
				expected = []string{"tool:shell", "agent:inherited"}
			case "system-inherited":
				in.SystemTask = " " + cadence.SystemTaskCatalogSync + " "
				in.PayloadPresent = false
				want.Target = cadence.TargetSystemTask
				want.SystemTask = cadence.SystemTaskCatalogSync
			case "tool-payload-ignored":
				in.Current.Target = cadence.TargetTool
			}
			service := appschedule.NewAdmission(appschedule.AdmissionHost{Agent: func(ref string) (roster.Profile, bool) { calls = append(calls, "agent:"+ref); return profile, true }, Workflow: func(ref string) bool { calls = append(calls, "workflow:"+ref); return true }, Tool: func(ref string) bool { calls = append(calls, "tool:"+ref); return true }})
			out, err := service.EditTarget(in)
			if err != nil || !reflect.DeepEqual(out, want) || !reflect.DeepEqual(calls, expected) {
				t.Fatal(out, want, err, calls, expected)
			}
		})
	}
}
func TestScheduleEditTargetRetainsAgentTargetPolicyPayloadErrorOrdering(t *testing.T) {
	no := false
	for _, mode := range []string{"current-system-payload", "new-system-payload", "system-agent", "missing-agent", "retired-agent", "paused-agent", "managed-agent", "all-targets", "system-workflow", "missing-workflow", "workflow-payload", "empty-tool", "missing-tool", "inherited-missing", "inherited-denied", "tool-payload", "system-unknown", "unknown-target"} {
		t.Run(mode, func(t *testing.T) {
			in := appschedule.EditTargetInput{}
			profile := roster.Profile{Slug: "canonical", Enabled: true}
			want := ""
			calls := []string{}
			expected := []string{}
			switch mode {
			case "current-system-payload":
				in.Current.Target = cadence.TargetSystemTask
				in.PayloadPresent = true
				in.Agent = "missing"
				in.AgentPresent = true
				want = "system task schedules do not accept args.payload"
			case "new-system-payload":
				in.SystemTask = "bad"
				in.PayloadPresent = true
				in.Agent = "missing"
				in.AgentPresent = true
				want = "system task schedules do not accept args.payload"
			case "system-agent":
				in.SystemTask = "bad"
				in.Agent = "missing"
				in.AgentPresent = true
				want = "system task schedules cannot also set args.agent"
			case "missing-agent", "retired-agent", "paused-agent", "managed-agent":
				in.Target = "unknown"
				in.Agent = " alias "
				in.AgentPresent = true
				expected = []string{"agent:alias"}
				switch mode {
				case "missing-agent":
					want = "unknown agent: alias"
				case "retired-agent":
					profile.Retired = true
					profile.Enabled = false
					want = "agent canonical is retired — revive it first"
				case "paused-agent":
					profile.Enabled = false
					want = "agent canonical is paused"
				case "managed-agent":
					profile.DirectCallable = &no
					want = "selected managed refusal"
					expected = append(expected, "managed:canonical:scheduled")
				}
			case "all-targets":
				in.Workflow = "flow"
				in.SystemTask = "bad"
				in.Tool = "shell"
				want = "tool schedules cannot also set args.workflow"
			case "system-workflow":
				in.Workflow = "flow"
				in.SystemTask = "bad"
				want = "system task schedules cannot also set args.workflow"
			case "missing-workflow", "workflow-payload":
				in.Workflow = "flow"
				in.PayloadPresent = true
				in.Payload = make(chan int)
				expected = []string{"workflow:flow"}
				if mode == "missing-workflow" {
					want = "unknown workflow: flow"
				} else {
					want = "payload must be JSON-serializable: json: unsupported type: chan int"
				}
			case "empty-tool":
				in.Target = cadence.TargetTool
				want = "args.tool required for tool schedules"
			case "missing-tool", "inherited-missing", "inherited-denied", "tool-payload":
				in.Tool = "shell"
				in.PayloadPresent = true
				in.Payload = make(chan int)
				expected = []string{"tool:shell"}
				switch mode {
				case "missing-tool":
					want = "unknown tool: shell"
				case "inherited-missing":
					in.Current.Agent = " inherited "
					expected = append(expected, "agent:inherited")
					want = "unknown agent: inherited"
				case "inherited-denied":
					in.Current.Agent = " inherited "
					profile.ToolDeny = []string{" SHELL "}
					expected = append(expected, "agent:inherited")
					want = "agent canonical cannot schedule tool shell: agent tool denylist"
				case "tool-payload":
					want = "payload must be JSON-serializable: json: unsupported type: chan int"
				}
			case "system-unknown":
				in.SystemTask = "bad"
				want = "unknown system task: bad"
			case "unknown-target":
				in.Target = "unknown"
				want = "unknown schedule target: unknown"
			}
			service := appschedule.NewAdmission(appschedule.AdmissionHost{Agent: func(ref string) (roster.Profile, bool) {
				calls = append(calls, "agent:"+ref)
				return profile, mode != "missing-agent" && mode != "inherited-missing"
			}, Workflow: func(ref string) bool { calls = append(calls, "workflow:"+ref); return mode != "missing-workflow" }, Tool: func(ref string) bool { calls = append(calls, "tool:"+ref); return mode != "missing-tool" }, ManagedDirectError: func(p roster.Profile, action string) string {
				calls = append(calls, "managed:"+p.Slug+":"+action)
				return "selected managed refusal"
			}})
			out, err := service.EditTarget(in)
			if err == nil || err.Error() != want || !reflect.DeepEqual(out, appschedule.EditTarget{}) || !reflect.DeepEqual(calls, expected) {
				t.Fatal(out, err, want, calls, expected)
			}
		})
	}
}
