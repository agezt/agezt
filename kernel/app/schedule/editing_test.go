// SPDX-License-Identifier: MIT

package schedule_test

import (
	"context"
	"encoding/json"
	"errors"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"reflect"
	"testing"
	"time"
)

type editProbe struct {
	*cadence.Store
	calls      []string
	causes     map[string]error
	falsePhase string
	getMiss    bool
}

func (p *editProbe) Get(id string) (cadence.Entry, bool) {
	p.calls = append(p.calls, "get")
	if p.getMiss {
		return cadence.Entry{}, false
	}
	return p.Store.Get(id)
}
func (p *editProbe) phase(name string) (bool, error) {
	p.calls = append(p.calls, name)
	if cause := p.causes[name]; cause != nil {
		return false, cause
	}
	if p.falsePhase == name {
		return false, nil
	}
	return true, nil
}
func (p *editProbe) SetIntent(id, value string) (bool, error) {
	ok, err := p.phase("intent")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetIntent(id, value)
}
func (p *editProbe) SetModel(id, value string) (bool, error) {
	ok, err := p.phase("model")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetModel(id, value)
}
func (p *editProbe) SetAgent(id, value string) (bool, error) {
	ok, err := p.phase("agent")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetAgent(id, value)
}
func (p *editProbe) SetWorkflowTarget(id, value string, payload json.RawMessage) (bool, error) {
	ok, err := p.phase("workflow")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetWorkflowTarget(id, value, payload)
}
func (p *editProbe) SetToolTarget(id, value string, payload json.RawMessage) (bool, error) {
	ok, err := p.phase("tool")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetToolTarget(id, value, payload)
}
func (p *editProbe) SetSystemTaskTarget(id, value string) (bool, error) {
	ok, err := p.phase("system")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetSystemTaskTarget(id, value)
}
func (p *editProbe) SetIntentTarget(id string) (bool, error) {
	ok, err := p.phase("intent-target")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetIntentTarget(id)
}
func (p *editProbe) Reschedule(id, mode string, interval time.Duration, at, end, days int, tz string, once, now time.Time) (bool, error) {
	ok, err := p.phase("reschedule")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.Reschedule(id, mode, interval, at, end, days, tz, once, now)
}
func editFixture(t *testing.T) (*editProbe, cadence.Entry) {
	t.Helper()
	store, err := cadence.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Add("initial", time.Hour, "initial-model", cadence.SourceOperator, time.Unix(500, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetAgent(entry.ID, "initial-agent"); err != nil {
		t.Fatal(err)
	}
	entry, _ = store.Get(entry.ID)
	return &editProbe{Store: store, causes: map[string]error{}}, entry
}
func TestScheduleEditingBeginRetainsMissingShortCircuitClockOrderAndWirePresence(t *testing.T) {
	p, entry := editFixture(t)
	clockCalls := 0
	service := appschedule.NewEditing(p, appschedule.EditingHost{Now: func() time.Time { p.calls = append(p.calls, "clock"); clockCalls++; return time.Unix(1000, 0) }})
	state, found := service.Begin(context.Background(), appschedule.IDInput{ID: "missing"})
	raw, err := json.Marshal(appschedule.EditOutput{Updated: false})
	if found || !reflect.DeepEqual(state, appschedule.EditState{}) || clockCalls != 0 || string(raw) != `{"updated":false}` || err != nil || !reflect.DeepEqual(p.calls, []string{"get"}) {
		t.Fatal(state, found, clockCalls, string(raw), err, p.calls)
	}
	p.calls = nil
	state, found = service.Begin(context.Background(), appschedule.IDInput{ID: entry.ID})
	if !found || !reflect.DeepEqual(state.Current, entry) || !state.Now.Equal(time.Unix(1000, 0)) || clockCalls != 1 || !reflect.DeepEqual(p.calls, []string{"get", "clock"}) {
		t.Fatal(state, found, clockCalls, p.calls)
	}
}
func TestScheduleEditingRetainsPreflightAndLateErrorsOriginalCausesAndPartialUpdates(t *testing.T) {
	cause := errors.New("owned phase cause")
	for _, mode := range []string{"preflight", "intent", "missing-workflow", "workflow-payload", "workflow", "missing-tool", "tool-payload", "tool", "system", "intent-target", "late-timezone", "reschedule"} {
		t.Run(mode, func(t *testing.T) {
			p, entry := editFixture(t)
			in := appschedule.EditInput{ID: entry.ID, State: appschedule.EditState{Current: entry, Now: time.Unix(1000, 0)}, Intent: appschedule.EditField{Value: "changed", Present: true}, Model: appschedule.EditField{Value: "changed-model", Present: true}, AgentPresent: true, Target: appschedule.EditTarget{Agent: "changed-agent"}}
			wantErr := cause
			wantText := ""
			expected := []string{"intent", "model", "agent"}
			expectFields := true
			switch mode {
			case "preflight":
				in.Cadence.Interval = appschedule.CadenceNumber{Present: true, Err: cause}
				expected = nil
				expectFields = false
			case "intent":
				p.causes["intent"] = cause
				expected = []string{"intent"}
				expectFields = false
			case "missing-workflow", "workflow-payload", "workflow":
				in.Target.Target = cadence.TargetWorkflow
				in.Target.Workflow = "lookup-ref"
				expected = append(expected, "workflow-lookup")
				if mode == "missing-workflow" {
					wantErr = nil
					wantText = "unknown workflow: lookup-ref"
				} else if mode == "workflow-payload" {
					in.PayloadPresent = true
					in.Payload = make(chan int)
					wantErr = nil
					wantText = "payload must be JSON-serializable: json: unsupported type: chan int"
				} else {
					p.causes["workflow"] = cause
					expected = append(expected, "workflow")
				}
			case "missing-tool", "tool-payload", "tool":
				in.Target.Target = cadence.TargetTool
				in.Target.Tool = "shell"
				expected = append(expected, "tool-lookup")
				if mode == "missing-tool" {
					wantErr = nil
					wantText = "unknown tool: shell"
				} else if mode == "tool-payload" {
					in.PayloadPresent = true
					in.Payload = make(chan int)
					wantErr = nil
					wantText = "payload must be JSON-serializable: json: unsupported type: chan int"
				} else {
					p.causes["tool"] = cause
					expected = append(expected, "tool")
				}
			case "system":
				in.Target.Target = cadence.TargetSystemTask
				in.Target.SystemTask = cadence.SystemTaskCatalogSync
				p.causes["system"] = cause
				expected = append(expected, "system")
			case "intent-target":
				in.TargetPresent = true
				p.causes["intent-target"] = cause
				expected = append(expected, "intent-target")
			case "late-timezone":
				in.Target.Target = cadence.TargetTool
				in.Target.Tool = "shell"
				in.PayloadPresent = true
				in.Payload = map[string]any{"path": "owned"}
				in.TZErr = cause
				expected = append(expected, "tool-lookup", "tool")
			case "reschedule":
				in.Cadence.Interval = cadenceNumber(60)
				p.causes["reschedule"] = cause
				expected = append(expected, "reschedule")
			}
			service := appschedule.NewEditing(p, appschedule.EditingHost{WorkflowName: func(ref string) (string, bool) {
				p.calls = append(p.calls, "workflow-lookup")
				if ref != "lookup-ref" {
					t.Fatal(ref)
				}
				return "canonical-flow", mode != "missing-workflow"
			}, Tool: func(ref string) bool {
				p.calls = append(p.calls, "tool-lookup")
				if ref != "shell" {
					t.Fatal(ref)
				}
				return mode != "missing-tool"
			}})
			out, err := service.Apply(context.Background(), in)
			if err == nil || wantErr != nil && err != wantErr || wantText != "" && err.Error() != wantText || !reflect.DeepEqual(out, appschedule.EditOutput{}) || !reflect.DeepEqual(p.calls, expected) {
				t.Fatal(out, err, wantErr, wantText, p.calls, expected)
			}
			stored, _ := p.Store.Get(entry.ID)
			if expectFields {
				if stored.Intent != "changed" || stored.Agent != "changed-agent" {
					t.Fatal(stored)
				}
				if mode == "late-timezone" {
					if stored.Target != cadence.TargetTool || stored.Tool != "shell" || stored.Model != "" || string(stored.Payload) != `{"path":"owned"}` {
						t.Fatal(stored)
					}
				} else if stored.Model != "changed-model" {
					t.Fatal(stored)
				}
			} else if !reflect.DeepEqual(stored, entry) {
				t.Fatal("preflight/intent failure changed fixture", stored, entry)
			}
		})
	}
}
func TestScheduleEditingRetainsIgnoredFieldCausesFalseSettersAndFinalMissingProjection(t *testing.T) {
	for _, mode := range []string{"ignored-fields", "false-target", "false-reschedule", "missing-refetch"} {
		t.Run(mode, func(t *testing.T) {
			p, entry := editFixture(t)
			cause := errors.New("ignored owned cause")
			in := appschedule.EditInput{ID: entry.ID, State: appschedule.EditState{Current: entry, Now: time.Unix(1000, 0)}}
			expected := []string{"get"}
			switch mode {
			case "ignored-fields":
				p.causes["model"] = cause
				p.causes["agent"] = cause
				in.Model = appschedule.EditField{Value: "ignored-model", Present: true}
				in.AgentPresent = true
				in.Target.Agent = "ignored-agent"
				expected = []string{"model", "agent", "get"}
			case "false-target":
				p.falsePhase = "workflow"
				in.Target.Target = cadence.TargetWorkflow
				in.Target.Workflow = "lookup-ref"
				expected = []string{"workflow-lookup", "workflow", "get"}
			case "false-reschedule":
				p.falsePhase = "reschedule"
				in.Cadence.Interval = cadenceNumber(60)
				expected = []string{"reschedule", "get"}
			case "missing-refetch":
				p.getMiss = true
			}
			service := appschedule.NewEditing(p, appschedule.EditingHost{WorkflowName: func(string) (string, bool) {
				p.calls = append(p.calls, "workflow-lookup")
				return "canonical-flow", true
			}})
			out, err := service.Apply(context.Background(), in)
			want := appschedule.Project(entry)
			if mode == "missing-refetch" {
				want = appschedule.Project(cadence.Entry{})
			}
			if err != nil || !out.Updated || out.Record == nil || !reflect.DeepEqual(*out.Record, want) || !reflect.DeepEqual(p.calls, expected) {
				t.Fatal(out, err, want, p.calls, expected)
			}
		})
	}
}
func TestScheduleEditingRetainsActualFiveReschedulesFourTargetsAndFreshProjection(t *testing.T) {
	for _, mode := range []string{cadence.ModeInterval, cadence.ModeOnce, cadence.ModeContinuous, cadence.ModeWindow, cadence.ModeDaily} {
		for _, target := range []string{cadence.TargetIntent, cadence.TargetWorkflow, cadence.TargetSystemTask, cadence.TargetTool} {
			t.Run(mode+"/"+target, func(t *testing.T) {
				p, entry := editFixture(t)
				service := appschedule.NewEditing(p, appschedule.EditingHost{Now: func() time.Time { return time.Unix(1000, 0) }, WorkflowName: func(ref string) (string, bool) {
					p.calls = append(p.calls, "workflow-lookup")
					if ref != "lookup-ref" {
						t.Fatal(ref)
					}
					return "canonical-flow", true
				}, Tool: func(ref string) bool { p.calls = append(p.calls, "tool-lookup"); return ref == "shell" }})
				state, found := service.Begin(context.Background(), appschedule.IDInput{ID: entry.ID})
				if !found {
					t.Fatal("missing fixture")
				}
				p.calls = nil
				in := appschedule.EditInput{ID: entry.ID, State: state, Intent: appschedule.EditField{Value: "changed", Present: true}, Model: appschedule.EditField{Value: "changed-model", Present: true}, AgentPresent: true, TargetPresent: true, Target: appschedule.EditTarget{Target: target, Agent: "changed-agent", Workflow: "", Tool: "", SystemTask: ""}}
				switch target {
				case cadence.TargetWorkflow:
					in.Target.Workflow = "lookup-ref"
					in.PayloadPresent = true
					in.Payload = nil
				case cadence.TargetTool:
					in.Target.Tool = "shell"
					in.PayloadPresent = true
					in.Payload = map[string]any{"path": "owned"}
				case cadence.TargetSystemTask:
					in.Target.SystemTask = cadence.SystemTaskCatalogSync
					in.Target.Agent = ""
				}
				switch mode {
				case cadence.ModeInterval:
					in.Cadence.Interval = cadenceNumber(60.9)
				case cadence.ModeOnce:
					in.Cadence.OnceAt = cadenceNumber(2000.9)
				case cadence.ModeContinuous:
					in.Cadence.Cooldown = cadenceNumber(60.9)
				case cadence.ModeWindow:
					in.Cadence.WindowStart = cadenceNumber(60.9)
					in.Cadence.WindowEnd = cadenceNumber(120.9)
					in.Cadence.Interval = cadenceNumber(60.9)
					in.Cadence.Days = cadenceNumber(0.9)
					in.Cadence.TZ = "UTC"
					in.TZ = "UTC"
				case cadence.ModeDaily:
					in.Cadence.AtMinutes = cadenceNumber(60.9)
					in.Cadence.Days = cadenceNumber(0.9)
					in.Cadence.TZ = "UTC"
					in.TZ = "UTC"
				}
				out, err := service.Apply(context.Background(), in)
				stored, _ := p.Store.Get(entry.ID)
				expected := []string{"intent", "model", "agent"}
				switch target {
				case cadence.TargetIntent:
					expected = append(expected, "intent-target")
				case cadence.TargetWorkflow:
					expected = append(expected, "workflow-lookup", "workflow")
				case cadence.TargetTool:
					expected = append(expected, "tool-lookup", "tool")
				case cadence.TargetSystemTask:
					expected = append(expected, "system")
				}
				expected = append(expected, "reschedule", "get")
				if err != nil || !out.Updated || out.Record == nil || !reflect.DeepEqual(*out.Record, appschedule.Project(stored)) || stored.Mode != mode || stored.Target != target || stored.Intent != "changed" || stored.ID != entry.ID || stored.CreatedUnix != 500 || !reflect.DeepEqual(p.calls, expected) {
					t.Fatal(out, err, stored, p.calls, expected)
				}
				switch target {
				case cadence.TargetIntent:
					if stored.Model != "changed-model" || stored.Agent != "changed-agent" || stored.Workflow != "" || stored.Payload != nil {
						t.Fatal(stored)
					}
				case cadence.TargetWorkflow:
					if stored.Workflow != "canonical-flow" || string(stored.Payload) != "null" || stored.Agent != "changed-agent" {
						t.Fatal(stored)
					}
				case cadence.TargetTool:
					if stored.Tool != "shell" || stored.Model != "" || string(stored.Payload) != `{"path":"owned"}` {
						t.Fatal(stored)
					}
				case cadence.TargetSystemTask:
					if stored.SystemTask != cadence.SystemTaskCatalogSync || stored.Model != "" || stored.Agent != "" || stored.Payload != nil {
						t.Fatal(stored)
					}
				}
				switch mode {
				case cadence.ModeInterval:
					if stored.IntervalSec != 60 || stored.NextRunUnix != 1060 {
						t.Fatal(stored)
					}
				case cadence.ModeOnce:
					if stored.NextRunUnix != 2000 {
						t.Fatal(stored)
					}
				case cadence.ModeContinuous:
					if stored.IntervalSec != 60 {
						t.Fatal(stored)
					}
				case cadence.ModeWindow:
					if stored.IntervalSec != 60 || stored.AtMinutes != 60 || stored.EndMinutes != 120 || stored.Days != 0 || stored.TZ != "UTC" {
						t.Fatal(stored)
					}
				case cadence.ModeDaily:
					if stored.AtMinutes != 60 || stored.Days != 0 || stored.TZ != "UTC" {
						t.Fatal(stored)
					}
				}
			})
		}
	}
}
