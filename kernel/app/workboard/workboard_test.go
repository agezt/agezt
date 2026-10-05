// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"encoding/json"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/assure"
	"github.com/agezt/agezt/kernel/proof"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"reflect"
	"testing"
	"time"
)

func TestTaskProjectionRetainsComputedCountsProofAndRetryPresence(t *testing.T) {
	task := tasks.Task{ID: "owned", Title: "fixture", Status: tasks.StatusTriage, CreatedMS: 100, UpdatedMS: 200, Comments: []tasks.Comment{{ID: "comment", Body: "body"}}, Links: []tasks.Link{{ID: "link", Type: "artifact", Target: "owned"}}, Attempts: []tasks.Attempt{{ID: "one", Status: "failed"}, {ID: "two", Status: " STALE "}, {ID: "three", Status: "done"}}, Criteria: []proof.Criterion{{Text: "one"}, {Text: "two", Met: true}}, RetryPolicy: &tasks.RetryPolicy{MaxAttempts: 3}}
	out := appwork.Project(task)
	if out.CommentCount != 1 || out.LinkCount != 1 || out.AttemptCount != 3 || out.FailedAttemptCount != 2 || out.CriteriaCount == nil || *out.CriteriaCount != 2 || out.CriteriaMet == nil || *out.CriteriaMet != 1 || out.Gated == nil || !*out.Gated || out.Proven == nil || *out.Proven || out.MaxAttempts != 3 || out.NextAttempt != 3 {
		t.Fatalf("projection=%+v", out)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["id"] != "owned" || fields["created_ms"] != float64(100) || fields["updated_ms"] != float64(200) || fields["proven"] != false {
		t.Fatalf("actual task/false proof fields=%s", raw)
	}
	task.Proof = &proof.Proof{Verdict: assure.Verdict{Complete: true}, Criteria: []proof.Criterion{{Text: "one", Met: true}, {Text: "two", Met: true}}}
	out = appwork.Project(task)
	if out.Proven == nil || !*out.Proven {
		t.Fatalf("satisfied proof=%+v", out)
	}
	task.Attempts = append(task.Attempts, tasks.Attempt{Status: "failed"})
	out = appwork.Project(task)
	if out.NextAttempt != 0 || out.MaxAttempts != 3 {
		t.Fatalf("exhausted retry=%+v", out)
	}
	task.Criteria = nil
	task.RetryPolicy = nil
	out = appwork.Project(task)
	raw, err = json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	fields = map[string]any{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"criteria_count", "criteria_met", "gated", "proven", "max_attempts", "next_attempt"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("conditional %s unexpectedly present: %s", key, raw)
		}
	}
}

type workReader struct {
	rows   []tasks.Task
	filter tasks.Filter
	id     string
	found  bool
}

func (r *workReader) List(filter tasks.Filter) []tasks.Task { r.filter = filter; return r.rows }
func (r *workReader) Get(id string) (tasks.Task, bool) {
	r.id = id
	if len(r.rows) > 0 {
		return r.rows[0], r.found
	}
	return tasks.Task{}, r.found
}
func TestWorkboardReadsRetainFiltersLanesAndMissingErrors(t *testing.T) {
	r := &workReader{rows: []tasks.Task{{ID: "one", Assignee: " zed ", Status: tasks.StatusTriage}, {ID: "two", Assignee: "Alpha", Status: tasks.StatusTriage}, {ID: "three", Assignee: "", Status: tasks.StatusTriage}, {ID: "four", Assignee: "Alpha", Status: tasks.StatusBlocked}}, found: true}
	s := appwork.New(r)
	ctx := context.Background()
	in := appwork.ListInput{Status: tasks.StatusTriage, Tenant: "acme", Assignee: "Alpha", IncludeArchived: true, Limit: 7}
	out, err := s.List(ctx, in)
	want := tasks.Filter{Status: in.Status, Tenant: in.Tenant, Assignee: in.Assignee, IncludeArchived: true, Limit: 7}
	if err != nil || out.Count != 4 || out.Tasks[0].ID != "one" || !reflect.DeepEqual(r.filter, want) {
		t.Fatalf("list=%+v filter=%+v err=%v", out, r.filter, err)
	}
	lanes, err := s.Lanes(ctx, in)
	if err != nil || lanes.Count != 3 || lanes.TaskCount != 4 || lanes.Lanes[0].Assignee != "Alpha" || lanes.Lanes[0].Count != 2 || lanes.Lanes[0].Counts["blocked"] != 1 || lanes.Lanes[1].Assignee != "zed" || lanes.Lanes[2].Assignee != "" || lanes.Lanes[2].Label != "unassigned" || r.filter.Assignee != "" || r.filter.Limit != 7 || r.filter.Tenant != "acme" {
		t.Fatalf("lanes=%+v filter=%+v err=%v", lanes, r.filter, err)
	}
	shown, err := s.Show(ctx, appwork.ShowInput{ID: " raw-id "})
	if err != nil || shown.Task.ID != "one" || r.id != " raw-id " {
		t.Fatalf("show=%+v id=%q err=%v", shown, r.id, err)
	}
	if _, err := s.Show(ctx, appwork.ShowInput{}); err == nil || err.Error() != "workboard_show requires id" {
		t.Fatalf("missing input=%v", err)
	}
	r.found = false
	if _, err := s.Show(ctx, appwork.ShowInput{ID: "unknown"}); err == nil || err.Error() != "unknown workboard task: unknown" {
		t.Fatalf("missing task=%v", err)
	}
	r.rows = nil
	empty, err := s.List(ctx, appwork.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(empty)
	if err != nil || string(raw) != `{"tasks":[],"count":0}` {
		t.Fatalf("empty list=%s err=%v", raw, err)
	}
	emptyLanes, err := s.Lanes(ctx, appwork.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(emptyLanes)
	if err != nil || string(raw) != `{"lanes":[],"count":0,"task_count":0}` {
		t.Fatalf("empty lanes=%s err=%v", raw, err)
	}
}
func TestWorkboardReadServiceRetainsActualStoreSelection(t *testing.T) {
	store, err := tasks.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range []tasks.CreateSpec{{Title: "primary", Assignee: "writer"}, {Title: "tenant", Tenant: "acme", Assignee: "reviewer"}, {Title: "archived"}} {
		task, _, err := store.Create(spec, time.UnixMilli(100))
		if err != nil {
			t.Fatal(err)
		}
		if spec.Title == "archived" {
			if _, err := store.Archive(task.ID, "owner", time.UnixMilli(101)); err != nil {
				t.Fatal(err)
			}
		}
	}
	s := appwork.New(store)
	for _, tc := range []struct {
		in    appwork.ListInput
		count int
	}{{appwork.ListInput{Limit: 100}, 2}, {appwork.ListInput{Tenant: "acme", Limit: 100}, 1}, {appwork.ListInput{Assignee: "writer", Limit: 100}, 1}, {appwork.ListInput{IncludeArchived: true, Limit: 100}, 3}, {appwork.ListInput{IncludeArchived: true, Limit: 1}, 1}} {
		out, err := s.List(context.Background(), tc.in)
		if err != nil || out.Count != tc.count {
			t.Fatalf("store filter %+v out=%+v err=%v", tc.in, out, err)
		}
	}
}
