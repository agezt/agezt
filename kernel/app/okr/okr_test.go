// SPDX-License-Identifier: MIT
package okr_test

import (
	"context"
	"encoding/json"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	workapi "github.com/agezt/agezt/kernel/modules/work/api"
	objectives "github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type reader struct {
	rows   []objectives.Objective
	filter objectives.Filter
	id     string
	found  bool
}

func (r *reader) List(filter objectives.Filter) []objectives.Objective {
	r.filter = filter
	return r.rows
}
func (r *reader) Get(id string) (objectives.Objective, bool) {
	r.id = id
	if len(r.rows) == 0 {
		return objectives.Objective{}, false
	}
	return r.rows[0], r.found
}

// library is a stub of the WORK MODULE's objective service — the face the
// application binds since W3.76r (the daemon's own spellings of these verbs
// are gone).
type library struct {
	calls     []string
	progress  objectives.ObjectiveProgress
	objective objectives.Objective
	cause     error
	method    string
	args      []any
}

func (l *library) Create(corr string, spec objectives.CreateSpec) (objectives.Objective, error) {
	l.method = "create"
	l.args = []any{corr, spec}
	return l.objective, l.cause
}
func (l *library) AddKeyResult(corr, id, title string, target int) (objectives.Objective, error) {
	l.method = "keyresult"
	l.args = []any{corr, id, title, target}
	return l.objective, l.cause
}
func (l *library) LinkTask(corr, id, kr, task string) (objectives.Objective, error) {
	l.method = "link"
	l.args = []any{corr, id, kr, task}
	return l.objective, l.cause
}
func (l *library) UnlinkTask(corr, id, kr, task string) (objectives.Objective, error) {
	l.method = "unlink"
	l.args = []any{corr, id, kr, task}
	return l.objective, l.cause
}
func (l *library) Archive(corr, id string) (objectives.Objective, error) {
	l.method = "archive"
	l.args = []any{corr, id}
	return l.objective, l.cause
}
func (l *library) Progress(o objectives.Objective) objectives.ObjectiveProgress {
	l.calls = append(l.calls, o.ID)
	return l.progress
}
func (l *library) ProgressOf(id string) (objectives.Objective, objectives.ObjectiveProgress, bool) {
	return l.objective, l.progress, l.objective.ID == id
}
func (l *library) RecomputeForTask(corr, taskID string) {}

// The work module's service over a daemon-held library and board — the shape
// the composition root builds (W3.76r).
func objectiveLibrary(k *runtime.Kernel) workapi.ObjectiveService {
	return k.Objectives()
}

func TestOKRReadsPreserveFilterAndLiveProjectionShape(t *testing.T) {
	objective := objectives.Objective{ID: "owned", Title: "title", Description: "description", Owner: "writer", Tenant: "team", Status: objectives.StatusActive, KeyResults: []objectives.KeyResult{{ID: "kr", Title: "criterion", Target: 2, TaskIDs: []string{"task"}, CreatedMS: 100}}, CreatedMS: 100, UpdatedMS: 101}
	store := &reader{rows: []objectives.Objective{objective}, found: true}
	progress := &library{progress: objectives.ObjectiveProgress{ObjectiveID: "owned", KeyResults: []objectives.KeyResultProgress{{ID: "kr", Done: 1, Total: 1, Target: 2, Percent: 50}}, Percent: 50, Achieved: false}}
	service := appokr.New(store, progress)
	input := appokr.ListInput{Status: "unknown", Tenant: "team", IncludeArchived: true, Limit: 3}
	out, err := service.List(context.Background(), input)
	if err != nil || out.Count != 1 || !reflect.DeepEqual(store.filter, objectives.Filter{Status: "unknown", Tenant: "team", IncludeArchived: true, Limit: 3}) || !reflect.DeepEqual(out.Objectives[0].Objective, objective) || out.Objectives[0].Percent != 50 || out.Objectives[0].Achieved || out.Objectives[0].KeyResultCount != 1 || !reflect.DeepEqual(out.Objectives[0].Progress, progress.progress) {
		t.Fatal(out, err, store.filter)
	}
	progress.progress.Percent = 100
	progress.progress.Achieved = true
	shown, err := service.Show(context.Background(), appokr.ShowInput{ID: "alias"})
	if err != nil || store.id != "alias" || shown.Objective.Percent != 100 || !shown.Objective.Achieved || shown.Objective.Status != objectives.StatusActive || !reflect.DeepEqual(progress.calls, []string{"owned", "owned"}) {
		t.Fatal(shown, err, store.id, progress.calls)
	}
	raw, err := json.Marshal(shown)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err = json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	record := root["objective"].(map[string]any)
	if record["percent"] != float64(100) || record["achieved"] != true || record["key_result_count"] != float64(1) || record["progress"] == nil {
		t.Fatal(string(raw))
	}
}
func TestOKRReadsPreserveEmptyArraysRequiredZeroFieldsAndErrors(t *testing.T) {
	store := &reader{found: true}
	progress := &library{progress: objectives.ObjectiveProgress{KeyResults: []objectives.KeyResultProgress{}}}
	service := appokr.New(store, progress)
	out, err := service.List(context.Background(), appokr.ListInput{})
	raw, _ := json.Marshal(out)
	if err != nil || out.Objectives == nil || string(raw) != "{\"objectives\":[],\"count\":0}" {
		t.Fatal(out, err, string(raw))
	}
	if _, err := service.Show(context.Background(), appokr.ShowInput{}); err == nil || err.Error() != "okr_show requires id" || store.id != "" {
		t.Fatal(err, store.id)
	}
	if _, err := service.Show(context.Background(), appokr.ShowInput{ID: "missing"}); err == nil || err.Error() != "unknown objective: missing" {
		t.Fatal(err)
	}
	record := service.Project(objectives.Objective{ID: "zero"})
	raw, _ = json.Marshal(record)
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"percent", "achieved", "key_result_count", "progress"} {
		if _, exists := values[key]; !exists {
			t.Fatal("required zero/false field missing", key, string(raw))
		}
	}
}
func TestOKRReadsUseActualStoreAndLiveWorkboardRollup(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(withWorkboard(t, runtime.Config{BaseDir: dir, Provider: mock.New()}))
	if err != nil {
		t.Fatal(err)
	}
	k.SetBoardRunner(workboard.NewRunner(workboard.NewService(k.Workboard(), k.WorkboardDependencies())))
	// The objective verbs live in the WORK MODULE (W3.76r): this test is such a
	// host — it builds the module's service over the library it opened.
	library := objectiveLibrary(k)
	defer k.Close()
	objective, err := library.Create("owned-corr", objectives.CreateSpec{Title: "owned", Tenant: "team"})
	if err != nil {
		t.Fatal(err)
	}
	objective, err = library.AddKeyResult("owned-corr", objective.ID, "criterion", 2)
	if err != nil {
		t.Fatal(err)
	}
	kr := objective.KeyResults[0].ID
	ids := []string{}
	for _, title := range []string{"first", "second"} {
		task, _, err := k.Workboard().Create(workboard.CreateSpec{Title: title}, time.UnixMilli(100))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, task.ID)
		if _, err = library.LinkTask("owned-corr", objective.ID, kr, task.ID); err != nil {
			t.Fatal(err)
		}
	}
	service := appokr.New(k.OKR(), library)
	for index, want := range []int{0, 50, 100} {
		if index > 0 {
			if _, err = k.BoardRunner().CompleteWorkboardTask("owned-corr", ids[index-1], "writer"); err != nil {
				t.Fatal(err)
			}
		}
		out, err := service.Show(context.Background(), appokr.ShowInput{ID: objective.ID})
		if err != nil || out.Objective.Percent != want || out.Objective.Achieved != (want == 100) || out.Objective.Progress.KeyResults[0].Done != index {
			t.Fatal(out, err)
		}
	}
	if _, err = library.UnlinkTask("owned-corr", objective.ID, kr, ids[1]); err != nil {
		t.Fatal(err)
	}
	out, err := service.List(context.Background(), appokr.ListInput{Tenant: "team", IncludeArchived: true})
	if err != nil || out.Count != 1 || out.Objectives[0].Percent != 50 || out.Objectives[0].Achieved || out.Objectives[0].Status != objectives.StatusAchieved {
		t.Fatalf("live rollup must not be replaced by cached status: %+v err=%v", out, err)
	}
}
