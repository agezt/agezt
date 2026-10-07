// SPDX-License-Identifier: MIT
package okr_test

import (
	"context"
	"encoding/json"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	objectives "github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workboard"
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

type rollup struct {
	calls    []string
	progress objectives.ObjectiveProgress
}

func (r *rollup) Rollup(o objectives.Objective) objectives.ObjectiveProgress {
	r.calls = append(r.calls, o.ID)
	return r.progress
}
func TestOKRReadsPreserveFilterAndLiveProjectionShape(t *testing.T) {
	objective := objectives.Objective{ID: "owned", Title: "title", Description: "description", Owner: "writer", Tenant: "team", Status: objectives.StatusActive, KeyResults: []objectives.KeyResult{{ID: "kr", Title: "criterion", Target: 2, TaskIDs: []string{"task"}, CreatedMS: 100}}, CreatedMS: 100, UpdatedMS: 101}
	store := &reader{rows: []objectives.Objective{objective}, found: true}
	progress := &rollup{progress: objectives.ObjectiveProgress{ObjectiveID: "owned", KeyResults: []objectives.KeyResultProgress{{ID: "kr", Done: 1, Total: 1, Target: 2, Percent: 50}}, Percent: 50, Achieved: false}}
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
	progress := &rollup{progress: objectives.ObjectiveProgress{KeyResults: []objectives.KeyResultProgress{}}}
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
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	objective, err := k.CreateObjective("owned-corr", objectives.CreateSpec{Title: "owned", Tenant: "team"})
	if err != nil {
		t.Fatal(err)
	}
	objective, err = k.AddObjectiveKeyResult("owned-corr", objective.ID, "criterion", 2)
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
		if _, err = k.LinkObjectiveTask("owned-corr", objective.ID, kr, task.ID); err != nil {
			t.Fatal(err)
		}
	}
	service := appokr.New(k.OKR(), k)
	for index, want := range []int{0, 50, 100} {
		if index > 0 {
			if _, err = k.CompleteWorkboardTask("owned-corr", ids[index-1], "writer"); err != nil {
				t.Fatal(err)
			}
		}
		out, err := service.Show(context.Background(), appokr.ShowInput{ID: objective.ID})
		if err != nil || out.Objective.Percent != want || out.Objective.Achieved != (want == 100) || out.Objective.Progress.KeyResults[0].Done != index {
			t.Fatal(out, err)
		}
	}
	if _, err = k.UnlinkObjectiveTask("owned-corr", objective.ID, kr, ids[1]); err != nil {
		t.Fatal(err)
	}
	out, err := service.List(context.Background(), appokr.ListInput{Tenant: "team", IncludeArchived: true})
	if err != nil || out.Count != 1 || out.Objectives[0].Percent != 50 || out.Objectives[0].Achieved || out.Objectives[0].Status != objectives.StatusAchieved {
		t.Fatalf("live rollup must not be replaced by cached status: %+v err=%v", out, err)
	}
}
