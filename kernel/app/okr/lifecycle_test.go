// SPDX-License-Identifier: MIT
package okr_test

import (
	"context"
	"errors"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	"github.com/agezt/agezt/kernel/event"
	objectives "github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type mutationHost struct {
	method    string
	args      []any
	objective objectives.Objective
	cause     error
}

func (h *mutationHost) CreateObjective(corr string, spec objectives.CreateSpec) (objectives.Objective, error) {
	h.method = "create"
	h.args = []any{corr, spec}
	return h.objective, h.cause
}
func (h *mutationHost) AddObjectiveKeyResult(corr, id, title string, target int) (objectives.Objective, error) {
	h.method = "keyresult"
	h.args = []any{corr, id, title, target}
	return h.objective, h.cause
}
func (h *mutationHost) LinkObjectiveTask(corr, id, kr, task string) (objectives.Objective, error) {
	h.method = "link"
	h.args = []any{corr, id, kr, task}
	return h.objective, h.cause
}
func (h *mutationHost) UnlinkObjectiveTask(corr, id, kr, task string) (objectives.Objective, error) {
	h.method = "unlink"
	h.args = []any{corr, id, kr, task}
	return h.objective, h.cause
}
func (h *mutationHost) ArchiveObjective(corr, id string) (objectives.Objective, error) {
	h.method = "archive"
	h.args = []any{corr, id}
	return h.objective, h.cause
}
func TestOKRLifecycleRetainsActualPortInputsReturnedObjectAndErrorCause(t *testing.T) {
	for _, name := range []string{"create", "keyresult", "link", "unlink", "archive"} {
		for _, failed := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/success", true: "/cause"}[failed], func(t *testing.T) {
				cause := errors.New("owned cause")
				host := &mutationHost{objective: objectives.Objective{ID: "returned", Title: "title", Status: objectives.StatusActive, KeyResults: []objectives.KeyResult{{ID: "returned-kr"}}}}
				if failed {
					host.cause = cause
				}
				progress := &rollup{progress: objectives.ObjectiveProgress{ObjectiveID: "returned", Percent: 75, Achieved: true}}
				service := appokr.NewLifecycle(host, appokr.New(&reader{}, progress))
				ctx := context.Background()
				spec := objectives.CreateSpec{Title: "raw title", Description: "description", Owner: "owner", Tenant: "team"}
				var out appokr.ObjectiveOutput
				var err error
				var want []any
				switch name {
				case "create":
					out, err = service.Create(ctx, appokr.CreateInput{CorrelationID: "corr", Spec: spec})
					want = []any{"corr", spec}
				case "keyresult":
					out, err = service.KeyResult(ctx, appokr.KeyResultInput{CorrelationID: "corr", ID: "input", Title: "raw title", Target: -2})
					want = []any{"corr", "input", "raw title", -2}
				case "link":
					out, err = service.Link(ctx, appokr.LinkInput{CorrelationID: "corr", ID: "input", KeyResult: "kr", Task: "task"})
					want = []any{"corr", "input", "kr", "task"}
				case "unlink":
					out, err = service.Unlink(ctx, appokr.LinkInput{CorrelationID: "corr", ID: "input", KeyResult: "kr", Task: "task"})
					want = []any{"corr", "input", "kr", "task"}
				case "archive":
					out, err = service.Archive(ctx, appokr.ArchiveInput{CorrelationID: "corr", ID: "input"})
					want = []any{"corr", "input"}
				}
				if host.method != name || !reflect.DeepEqual(host.args, want) {
					t.Fatal(host.method, host.args, want)
				}
				if failed {
					if err != cause || !reflect.DeepEqual(out, appokr.ObjectiveOutput{}) || len(progress.calls) != 0 {
						t.Fatal("failed mutation projected partial output", out, err, progress.calls)
					}
					return
				}
				if err != nil || !reflect.DeepEqual(out.Objective.Objective, host.objective) || out.Objective.Percent != 75 || !out.Objective.Achieved || out.Objective.KeyResultCount != 1 || !reflect.DeepEqual(progress.calls, []string{"returned"}) {
					t.Fatal(out, err, progress.calls)
				}
			})
		}
	}
}
func TestOKRLifecycleUsesActualKernelJournalingAndPreservesReturnedSnapshot(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	service := appokr.NewLifecycle(k, appokr.New(k.OKR(), k))
	ctx := context.Background()
	out, err := service.Create(ctx, appokr.CreateInput{CorrelationID: "owned", Spec: objectives.CreateSpec{Title: "objective", Owner: "writer", Tenant: "team"}})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Objective.ID
	out, err = service.KeyResult(ctx, appokr.KeyResultInput{CorrelationID: "owned", ID: id, Title: "criterion", Target: 1})
	if err != nil {
		t.Fatal(err)
	}
	kr := out.Objective.KeyResults[0].ID
	task, _, err := k.Workboard().Create(workboard.CreateSpec{Title: "already done"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = k.Workboard().Complete(task.ID, "writer", time.UnixMilli(101)); err != nil {
		t.Fatal(err)
	}
	out, err = service.Link(ctx, appokr.LinkInput{CorrelationID: "owned", ID: id, KeyResult: kr, Task: task.ID})
	stored, _ := k.OKR().Get(id)
	if err != nil || out.Objective.Percent != 100 || !out.Objective.Achieved || out.Objective.Status != objectives.StatusActive || stored.Status != objectives.StatusAchieved {
		t.Fatalf("returned snapshot must not be refetched after recompute: out=%+v stored=%+v err=%v", out, stored, err)
	}
	out, err = service.Unlink(ctx, appokr.LinkInput{CorrelationID: "owned", ID: id, KeyResult: kr, Task: task.ID})
	if err != nil || out.Objective.Percent != 0 || out.Objective.Achieved || len(out.Objective.KeyResults[0].TaskIDs) != 0 {
		t.Fatal(out, err)
	}
	out, err = service.Archive(ctx, appokr.ArchiveInput{CorrelationID: "owned", ID: id})
	if err != nil || out.Objective.Status != objectives.StatusArchived {
		t.Fatal(out, err)
	}
	actions := map[string]int{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Subject == "okr."+id && (e.Kind == event.KindOKRObjectiveCreated || e.Kind == event.KindOKRObjectiveUpdated || e.Kind == event.KindOKRObjectiveAchieved) {
			if e.CorrelationID != "owned" {
				t.Fatal("domain correlation changed", e.CorrelationID)
			}
			actions[string(e.Kind)]++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if actions[string(event.KindOKRObjectiveCreated)] != 1 || actions[string(event.KindOKRObjectiveUpdated)] != 4 || actions[string(event.KindOKRObjectiveAchieved)] != 1 {
		t.Fatal("kernel domain journaling changed", actions)
	}
	causeOut, cause := service.Archive(ctx, appokr.ArchiveInput{CorrelationID: "owned", ID: "missing"})
	if !errors.Is(cause, objectives.ErrNotFound) || !reflect.DeepEqual(causeOut, appokr.ObjectiveOutput{}) {
		t.Fatal(causeOut, cause)
	}
}
