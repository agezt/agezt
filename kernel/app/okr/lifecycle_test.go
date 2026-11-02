// SPDX-License-Identifier: MIT
package okr_test

import (
	"context"
	"errors"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	"github.com/agezt/agezt/kernel/event"
	objectives "github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

func TestOKRLifecycleRetainsActualPortInputsReturnedObjectAndErrorCause(t *testing.T) {
	for _, name := range []string{"create", "keyresult", "link", "unlink", "archive"} {
		for _, failed := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/success", true: "/cause"}[failed], func(t *testing.T) {
				cause := errors.New("owned cause")
				library := &library{objective: objectives.Objective{ID: "returned", Title: "title", Status: objectives.StatusActive, KeyResults: []objectives.KeyResult{{ID: "returned-kr"}}}}
				if failed {
					library.cause = cause
				}
				library.progress = objectives.ObjectiveProgress{ObjectiveID: "returned", Percent: 75, Achieved: true}
				service := appokr.NewLifecycle(library, appokr.New(&reader{}, library))
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
				if library.method != name || !reflect.DeepEqual(library.args, want) {
					t.Fatal(library.method, library.args, want)
				}
				if failed {
					if err != cause || !reflect.DeepEqual(out, appokr.ObjectiveOutput{}) || len(library.calls) != 0 {
						t.Fatal("failed mutation projected partial output", out, err, library.calls)
					}
					return
				}
				if err != nil || !reflect.DeepEqual(out.Objective.Objective, library.objective) || out.Objective.Percent != 75 || !out.Objective.Achieved || out.Objective.KeyResultCount != 1 || !reflect.DeepEqual(library.calls, []string{"returned"}) {
					t.Fatal(out, err, library.calls)
				}
			})
		}
	}
}
func TestOKRLifecycleUsesActualKernelJournalingAndPreservesReturnedSnapshot(t *testing.T) {
	k, err := runtime.Open(withWorkboard(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	// The objective verbs live in the WORK MODULE (W3.76r): this test is such a
	// host — it builds the module's service over the library it opened.
	library := objectiveLibrary(k)
	service := appokr.NewLifecycle(library, appokr.New(k.OKR(), library))
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

// A daemon with no objective service refuses every write (W3.76r): the
// application asks the MODULE, and a module that was never wired has no
// service to ask.
func TestOKRLifecycleRefusesWithoutTheModuleService(t *testing.T) {
	service := appokr.NewLifecycle(nil, appokr.New(&reader{}, nil))
	if _, err := service.Create(context.Background(), appokr.CreateInput{Spec: objectives.CreateSpec{Title: "t"}}); err == nil {
		t.Fatal("the application wrote an objective with no module service")
	}
}
