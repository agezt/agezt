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

type creationProbe struct {
	*cadence.Store
	calls                           []string
	phase                           string
	getMiss                         bool
	cause, createCause, removeCause error
	createdID                       string
}

func (p *creationProbe) Add(intent string, interval time.Duration, model, source string, now time.Time) (cadence.Entry, error) {
	p.calls = append(p.calls, "create")
	if p.createCause != nil {
		return cadence.Entry{ID: "partial"}, p.createCause
	}
	e, err := p.Store.Add(intent, interval, model, source, now)
	p.createdID = e.ID
	return e, err
}
func (p *creationProbe) binding(phase string) (bool, error) {
	p.calls = append(p.calls, phase)
	if p.phase == phase {
		return false, p.cause
	}
	return true, nil
}
func (p *creationProbe) SetAgent(id, agent string) (bool, error) {
	ok, err := p.binding("agent")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetAgent(id, agent)
}
func (p *creationProbe) SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error) {
	ok, err := p.binding("workflow")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetWorkflowTarget(id, ref, payload)
}
func (p *creationProbe) SetSystemTaskTarget(id, task string) (bool, error) {
	ok, err := p.binding("system task")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetSystemTaskTarget(id, task)
}
func (p *creationProbe) SetToolTarget(id, tool string, payload json.RawMessage) (bool, error) {
	ok, err := p.binding("tool")
	if !ok || err != nil {
		return ok, err
	}
	return p.Store.SetToolTarget(id, tool, payload)
}
func (p *creationProbe) Remove(id string) (bool, error) {
	p.calls = append(p.calls, "remove")
	if id != p.createdID {
		return false, errors.New("wrong rollback id")
	}
	if p.removeCause != nil {
		return false, p.removeCause
	}
	return p.Store.Remove(id)
}
func (p *creationProbe) Get(id string) (cadence.Entry, bool) {
	p.calls = append(p.calls, "get")
	if p.getMiss {
		return cadence.Entry{}, false
	}
	return p.Store.Get(id)
}
func TestScheduleCreationRetainsFourBindingFailureCompensationAndOriginalCause(t *testing.T) {
	cause, rollbackCause := errors.New("owned binding cause"), errors.New("owned rollback cause")
	for _, phase := range []string{"agent", "workflow", "system task", "tool"} {
		for _, missing := range []bool{false, true} {
			for _, rollbackFails := range []bool{false, true} {
				t.Run(phase+map[bool]string{false: " cause", true: " missing"}[missing]+map[bool]string{false: " rollback", true: " rollback-failure"}[rollbackFails], func(t *testing.T) {
					store, err := cadence.OpenStore(t.TempDir())
					if err != nil {
						t.Fatal(err)
					}
					probe := &creationProbe{Store: store, phase: phase, cause: cause}
					if missing {
						probe.cause = nil
					}
					if rollbackFails {
						probe.removeCause = rollbackCause
					}
					plan := appschedule.AddTarget{Intent: "owned", Agent: "canonical", Workflow: "owned-flow", SystemTask: cadence.SystemTaskCatalogSync, Tool: "shell", WorkflowPayload: json.RawMessage(`null`), ToolPayload: json.RawMessage(`{"a":1}`)}
					expected := []string{"create", "agent"}
					switch phase {
					case "workflow":
						plan.Target = cadence.TargetWorkflow
						expected = append(expected, "workflow")
					case "system task":
						plan.Agent = ""
						plan.Target = cadence.TargetSystemTask
						expected = []string{"create", "system task"}
					case "tool":
						plan.Target = cadence.TargetTool
						expected = append(expected, "tool")
					}
					expected = append(expected, "remove")
					out, err := appschedule.NewCreation(probe, func() time.Time { return time.Unix(1000, 0) }).Create(context.Background(), appschedule.CreateInput{Target: plan, Cadence: appschedule.CreateCadence{Seconds: 60}})
					want := "schedule disappeared before " + phase + " binding"
					if !missing {
						want = cause.Error()
					}
					if err == nil || err.Error() != want || !missing && err != cause || !reflect.DeepEqual(out, appschedule.Record{}) || !reflect.DeepEqual(probe.calls, expected) {
						t.Fatal(out, err, want, probe.calls, expected)
					}
					rows := store.List()
					if (len(rows) == 1) != rollbackFails {
						t.Fatal("best-effort rollback", rows, rollbackFails)
					}
					if len(rows) == 1 && rows[0].ID != probe.createdID {
						t.Fatal(rows, probe.createdID)
					}
				})
			}
		}
	}
}
func TestScheduleCreationRetainsInitialCauseAndValidationBeforeClockOrStore(t *testing.T) {
	store, err := cadence.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("owned creation cause")
	probe := &creationProbe{Store: store, createCause: cause}
	clockCalls := 0
	service := appschedule.NewCreation(probe, func() time.Time { clockCalls++; return time.Unix(1000, 0) })
	out, err := service.Create(context.Background(), appschedule.CreateInput{Target: appschedule.AddTarget{Intent: "owned", Agent: "canonical"}, Cadence: appschedule.CreateCadence{Seconds: 60}})
	if err != cause || !reflect.DeepEqual(out, appschedule.Record{}) || !reflect.DeepEqual(probe.calls, []string{"create"}) || clockCalls != 1 {
		t.Fatal(out, err, probe.calls, clockCalls)
	}
	for _, tc := range []struct{ mode, want string }{{cadence.ModeInterval, "args.interval_sec must be >= 1 (or pass at_minutes)"}, {cadence.ModeContinuous, "args.cooldown_sec must be >= 1"}, {"unknown", "unknown schedule cadence mode: unknown"}} {
		probe.calls = nil
		clockCalls = 0
		out, err := service.Create(context.Background(), appschedule.CreateInput{Target: appschedule.AddTarget{Intent: "owned"}, Cadence: appschedule.CreateCadence{Mode: tc.mode, Seconds: 0.9}})
		if err == nil || err.Error() != tc.want || !reflect.DeepEqual(out, appschedule.Record{}) || len(probe.calls) != 0 || clockCalls != 0 {
			t.Fatal(tc, out, err, probe.calls, clockCalls)
		}
	}
}
func TestScheduleCreationRetainsActualFiveCadenceVariantsBindingsAndRefreshedProjection(t *testing.T) {
	for _, mode := range []string{cadence.ModeInterval, cadence.ModeOnce, cadence.ModeContinuous, cadence.ModeWindow, cadence.ModeDaily} {
		for _, target := range []string{cadence.TargetIntent, cadence.TargetWorkflow, cadence.TargetSystemTask, cadence.TargetTool} {
			t.Run(mode+"/"+target, func(t *testing.T) {
				store, err := cadence.OpenStore(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				plan := appschedule.AddTarget{Intent: " owned ", Model: " model ", Agent: "canonical", Target: target, Workflow: "owned-flow", SystemTask: cadence.SystemTaskCatalogSync, Tool: "shell", WorkflowPayload: json.RawMessage(`null`), ToolPayload: json.RawMessage(`{"a":1}`)}
				if target == cadence.TargetSystemTask {
					plan.Agent = ""
				}
				clockCalls := 0
				out, err := appschedule.NewCreation(store, func() time.Time { clockCalls++; return time.Unix(1000, 0) }).Create(context.Background(), appschedule.CreateInput{Target: plan, Cadence: appschedule.CreateCadence{Mode: mode, Seconds: 60.9, At: 60.9, End: 120.9, Days: 0.9, OnceAt: 2000.9, TZ: "UTC"}})
				if err != nil {
					t.Fatal(err)
				}
				entry, found := store.Get(out.ID)
				if !found || clockCalls != 1 || entry.Mode != mode || entry.Target != target || entry.Source != cadence.SourceOperator || !entry.Enabled || entry.CreatedUnix != 1000 || entry.Intent != "owned" || !reflect.DeepEqual(out, appschedule.Project(entry)) {
					t.Fatal(out, entry, found, clockCalls)
				}
				switch mode {
				case cadence.ModeInterval:
					if entry.IntervalSec != 60 || entry.NextRunUnix != 1060 {
						t.Fatal(entry)
					}
				case cadence.ModeOnce:
					if entry.NextRunUnix != 2000 {
						t.Fatal(entry)
					}
				case cadence.ModeContinuous:
					if entry.IntervalSec != 60 || entry.NextRunUnix != 1000 {
						t.Fatal(entry)
					}
				case cadence.ModeWindow:
					if entry.IntervalSec != 60 || entry.AtMinutes != 60 || entry.EndMinutes != 120 || entry.Days != 0 || entry.TZ != "UTC" {
						t.Fatal(entry)
					}
				case cadence.ModeDaily:
					if entry.AtMinutes != 60 || entry.Days != 0 || entry.TZ != "UTC" {
						t.Fatal(entry)
					}
				}
				switch target {
				case cadence.TargetIntent:
					if entry.Agent != "canonical" || entry.Model != "model" {
						t.Fatal(entry)
					}
				case cadence.TargetWorkflow:
					if entry.Workflow != "owned-flow" || string(entry.Payload) != "null" || entry.Agent != "canonical" {
						t.Fatal(entry)
					}
				case cadence.TargetSystemTask:
					if entry.SystemTask != cadence.SystemTaskCatalogSync || entry.Agent != "" || entry.Model != "" || entry.Payload != nil {
						t.Fatal(entry)
					}
				case cadence.TargetTool:
					if entry.Tool != "shell" || string(entry.Payload) != `{"a":1}` || entry.Model != "" || entry.Agent != "canonical" {
						t.Fatal(entry)
					}
				}
			})
		}
	}
}
func TestScheduleCreationRetainsLocalBindingProjectionWhenRefetchMisses(t *testing.T) {
	for _, target := range []string{cadence.TargetWorkflow, cadence.TargetSystemTask, cadence.TargetTool} {
		store, err := cadence.OpenStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		probe := &creationProbe{Store: store, getMiss: true}
		plan := appschedule.AddTarget{Intent: "owned", Model: "model", Agent: "canonical", Target: target, Workflow: "owned-flow", SystemTask: cadence.SystemTaskCatalogSync, Tool: "shell", WorkflowPayload: json.RawMessage(`null`), ToolPayload: json.RawMessage(`{"a":1}`)}
		out, err := appschedule.NewCreation(probe, func() time.Time { return time.Unix(1000, 0) }).Create(context.Background(), appschedule.CreateInput{Target: plan, Cadence: appschedule.CreateCadence{Seconds: 60}})
		if err != nil || out.Target != target || out.Agent != "canonical" || out.Model != "model" || out.ID != probe.createdID {
			t.Fatal(out, err, probe.calls)
		}
		switch target {
		case cadence.TargetWorkflow:
			if out.Workflow != plan.Workflow || string(out.Payload) != "null" {
				t.Fatal(out)
			}
		case cadence.TargetSystemTask:
			if out.SystemTask != plan.SystemTask {
				t.Fatal(out)
			}
		case cadence.TargetTool:
			if out.Tool != plan.Tool || string(out.Payload) != `{"a":1}` {
				t.Fatal(out)
			}
		}
	}
}

func TestScheduleCreationRetainsContinuousMinimumClamp(t *testing.T) {
	store, err := cadence.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out, err := appschedule.NewCreation(store, func() time.Time { return time.Unix(1000, 0) }).Create(context.Background(), appschedule.CreateInput{Target: appschedule.AddTarget{Intent: "owned"}, Cadence: appschedule.CreateCadence{Mode: cadence.ModeContinuous, Seconds: 1.9}})
	if err != nil || out.IntervalSec != int64(cadence.MinInterval/time.Second) || out.NextRunUnix != 1000 {
		t.Fatal(out, err)
	}
}
