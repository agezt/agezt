// SPDX-License-Identifier: MIT

package schedule_test

import (
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/roster"
	"reflect"
	"testing"
)

func TestScheduleAddTargetRetainsCanonicalBindingsPayloadPresenceAndIgnoredIntentPayload(t *testing.T) {
	for _, target := range []string{cadence.TargetIntent, cadence.TargetWorkflow, cadence.TargetSystemTask, cadence.TargetTool} {
		for _, presence := range []bool{false, true} {
			t.Run(target+map[bool]string{false: " absent", true: " present"}[presence], func(t *testing.T) {
				calls := []string{}
				service := appschedule.NewAdmission(appschedule.AdmissionHost{
					Agent: func(ref string) (roster.Profile, bool) {
						calls = append(calls, "agent:"+ref)
						return roster.Profile{Slug: "canonical", Enabled: true}, true
					},
					WorkflowName: func(ref string) (string, bool) { calls = append(calls, "workflow:"+ref); return "canonical-flow", true },
					Tool:         func(ref string) bool { calls = append(calls, "tool:"+ref); return true },
				})
				in := appschedule.AddTargetInput{Target: " " + target + " ", Model: " raw-model ", Payload: nil, PayloadPresent: presence}
				want := appschedule.AddTarget{Target: target, Model: in.Model}
				expected := []string{}
				switch target {
				case cadence.TargetIntent:
					in.Intent = " raw-intent "
					in.Agent = " alias "
					in.Payload = make(chan int)
					want.Intent = in.Intent
					want.Agent = "canonical"
					expected = []string{"agent:alias"}
				case cadence.TargetWorkflow:
					in.Workflow = " alias-flow "
					want.Workflow = "canonical-flow"
					want.Intent = "workflow canonical-flow"
					expected = []string{"workflow:alias-flow"}
					if presence {
						want.WorkflowPayload = []byte("null")
					}
				case cadence.TargetSystemTask:
					in.SystemTask = " " + cadence.SystemTaskCatalogSync + " "
					want.SystemTask = cadence.SystemTaskCatalogSync
					want.Intent = "system task " + cadence.SystemTaskCatalogSync
					if presence {
						out, err := service.AddTarget(in)
						if err == nil || err.Error() != "system task schedules do not accept args.payload" || !reflect.DeepEqual(out, appschedule.AddTarget{}) || len(calls) != 0 {
							t.Fatal(out, err, calls)
						}
						return
					}
				case cadence.TargetTool:
					in.Tool = " shell "
					in.Agent = " alias "
					in.Payload = map[string]any{"path": "owned"}
					want.Tool = "shell"
					want.Agent = "canonical"
					want.Intent = "tool shell"
					expected = []string{"tool:shell", "agent:alias", "agent:canonical"}
					if presence {
						want.ToolPayload = []byte(`{"path":"owned"}`)
					}
				}
				out, err := service.AddTarget(in)
				if err != nil || !reflect.DeepEqual(out, want) || !reflect.DeepEqual(calls, expected) {
					t.Fatal(out, want, err, calls, expected)
				}
			})
		}
	}
}
func TestScheduleAddTargetRetainsConflictErrorAndAdmissionOrdering(t *testing.T) {
	no := false
	for _, mode := range []string{"unknown-target", "empty-intent", "all-targets", "system-workflow", "system-agent", "system-unknown", "system-payload", "missing-workflow", "workflow-payload", "empty-tool", "missing-tool", "tool-payload", "missing-agent", "retired-agent", "paused-agent", "managed-agent", "denied-tool", "allow-refusal"} {
		t.Run(mode, func(t *testing.T) {
			in := appschedule.AddTargetInput{Target: cadence.TargetIntent, Intent: "owned"}
			profile := roster.Profile{Slug: "canonical", Enabled: true}
			want := ""
			calls := []string{}
			expected := []string{}
			switch mode {
			case "unknown-target":
				in.Target = "unknown"
				want = "unknown schedule target: unknown"
			case "empty-intent":
				in.Intent = " "
				want = "agent task text required for target=intent schedules"
			case "all-targets":
				in.Workflow = "flow"
				in.SystemTask = "bad"
				in.Tool = "shell"
				want = "tool schedules cannot also set args.workflow"
			case "system-workflow":
				in.Workflow = "flow"
				in.SystemTask = "bad"
				want = "system task schedules cannot also set args.workflow"
			case "system-agent":
				in.SystemTask = "bad"
				in.Agent = "missing"
				want = "system task schedules cannot also set args.agent"
			case "system-unknown":
				in.SystemTask = "bad"
				in.PayloadPresent = true
				want = "unknown system task: bad"
			case "system-payload":
				in.SystemTask = cadence.SystemTaskCatalogSync
				in.PayloadPresent = true
				want = "system task schedules do not accept args.payload"
			case "missing-workflow":
				in.Workflow = "flow"
				in.Agent = "missing"
				in.PayloadPresent = true
				in.Payload = make(chan int)
				want = "unknown workflow: flow"
				expected = []string{"workflow:flow"}
			case "workflow-payload":
				in.Workflow = "flow"
				in.Agent = "missing"
				in.PayloadPresent = true
				in.Payload = make(chan int)
				want = "payload must be JSON-serializable: json: unsupported type: chan int"
				expected = []string{"workflow:flow"}
			case "empty-tool":
				in.Target = cadence.TargetTool
				want = "args.tool required for tool schedules"
			case "missing-tool":
				in.Tool = "shell"
				in.Agent = "missing"
				in.PayloadPresent = true
				in.Payload = make(chan int)
				want = "unknown tool: shell"
				expected = []string{"tool:shell"}
			case "tool-payload":
				in.Tool = "shell"
				in.Agent = "missing"
				in.PayloadPresent = true
				in.Payload = make(chan int)
				want = "payload must be JSON-serializable: json: unsupported type: chan int"
				expected = []string{"tool:shell"}
			default:
				in.Agent = " alias "
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
				case "denied-tool", "allow-refusal":
					in.Tool = "shell"
					expected = []string{"tool:shell", "agent:alias", "agent:canonical"}
					if mode == "denied-tool" {
						profile.ToolAllow = []string{"shell"}
						profile.ToolDeny = []string{" SHELL "}
						want = "agent canonical cannot schedule tool shell: agent tool denylist"
					} else {
						profile.ToolAllow = []string{"read"}
						want = "agent canonical cannot schedule tool shell: not in agent tool allowlist"
					}
				}
			}
			service := appschedule.NewAdmission(appschedule.AdmissionHost{WorkflowName: func(ref string) (string, bool) {
				calls = append(calls, "workflow:"+ref)
				return "canonical-flow", mode != "missing-workflow"
			}, Tool: func(ref string) bool { calls = append(calls, "tool:"+ref); return mode != "missing-tool" }, Agent: func(ref string) (roster.Profile, bool) {
				calls = append(calls, "agent:"+ref)
				return profile, mode != "missing-agent"
			}, ManagedDirectError: func(p roster.Profile, action string) string {
				calls = append(calls, "managed:"+p.Slug+":"+action)
				return "selected managed refusal"
			}})
			out, err := service.AddTarget(in)
			if err == nil || err.Error() != want || !reflect.DeepEqual(out, appschedule.AddTarget{}) || !reflect.DeepEqual(calls, expected) {
				t.Fatal(out, err, want, calls, expected)
			}
		})
	}
}
