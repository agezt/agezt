// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"encoding/json"
	"errors"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/event"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"testing"
)

type watchStore struct {
	task       tasks.Task
	found      bool
	blocked    []tasks.DependencyState
	cause      error
	got, depID string
}

func (s *watchStore) Get(id string) (tasks.Task, bool) { s.got = id; return s.task, s.found }
func (s *watchStore) BlockingDependencies(id string) ([]tasks.DependencyState, error) {
	s.depID = id
	return s.blocked, s.cause
}

type watchJournal struct {
	rows  []event.Event
	cause error
}

func (j watchJournal) Range(fn func(*event.Event) error) error {
	for i := range j.rows {
		if err := fn(&j.rows[i]); err != nil {
			return err
		}
	}
	return j.cause
}
func TestWorkboardWatchRetainsRunPrecedenceAndTaskIdentity(t *testing.T) {
	store := &watchStore{found: true, task: tasks.Task{ID: "canonical", Claim: &tasks.Claim{RunID: " claim "}, Attempts: []tasks.Attempt{{RunID: "attempt", StartedMS: 50, FinishedMS: 100}}, Links: []tasks.Link{{Type: "RUN", Target: " link ", CreatedMS: 100}}}}
	s := appwork.NewWatch(store, nil)
	ctx := context.Background()
	for _, tc := range []struct {
		explicit string
		claim    *tasks.Claim
		want     string
	}{{"explicit", store.task.Claim, "explicit"}, {"", store.task.Claim, "claim"}, {"", nil, "link"}} {
		store.task.Claim = tc.claim
		out, err := s.Watch(ctx, appwork.WatchInput{ID: "raw", RunID: tc.explicit, Limit: 50})
		if err != nil || out.RunID != tc.want || store.got != "raw" || store.depID != "canonical" {
			t.Fatalf("run=%+v get=%q dependencies=%q err=%v", out, store.got, store.depID, err)
		}
	}
	store.task.Links = nil
	out, err := s.Watch(ctx, appwork.WatchInput{ID: "raw"})
	if err != nil || out.RunID != "attempt" {
		t.Fatalf("attempt fallback=%+v err=%v", out, err)
	}
	store.task.Attempts = nil
	out, err = s.Watch(ctx, appwork.WatchInput{ID: "raw"})
	if err != nil || out.RunID != "" || out.Events != nil || out.BlockedDependencies == nil {
		t.Fatalf("empty snapshot=%+v err=%v", out, err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["run_id"]; ok {
		t.Fatalf("empty run present=%s", raw)
	}
	if v, ok := fields["events"]; !ok || v != nil {
		t.Fatalf("nil events changed=%s", raw)
	}
	emptyReader := appwork.NewWatch(store, watchJournal{})
	emptyOut, emptyErr := emptyReader.Watch(ctx, appwork.WatchInput{ID: "raw"})
	if emptyErr != nil || emptyOut.Events != nil {
		t.Fatalf("empty journal events=%+v err=%v", emptyOut, emptyErr)
	}
	store.found = false
	if _, err := s.Watch(ctx, appwork.WatchInput{ID: "missing"}); err == nil || err.Error() != "unknown workboard task: missing" {
		t.Fatalf("missing task=%v", err)
	}
	if _, err := s.Watch(ctx, appwork.WatchInput{}); err == nil || err.Error() != "workboard_watch requires id" {
		t.Fatalf("missing id=%v", err)
	}
}
func TestWorkboardWatchRetainsORFilterStableTailAndPayloadPresence(t *testing.T) {
	store := &watchStore{found: true, task: tasks.Task{ID: "owned"}, blocked: []tasks.DependencyState{{ID: "parent", Status: tasks.StatusBlocked, Title: "title", Missing: true, CreatedMS: 100}, {ID: "zero", Status: tasks.StatusTriage, CreatedMS: -1}}, cause: errors.New("legacy dependency partial error")}
	rows := []event.Event{{Seq: 4, Subject: "other", CorrelationID: "run", Kind: event.KindOpCompleted, Payload: json.RawMessage(`{}`)}, {Seq: 1, Subject: "workboard.owned", CorrelationID: "other", Kind: event.KindWorkboardTaskCreated, Payload: json.RawMessage(`{"id":"owned"}`)}, {Seq: 3, Subject: "workboard.owned", Kind: event.KindWorkboardTaskUpdated, Payload: json.RawMessage(`null`)}, {Seq: 2, Subject: "unrelated", CorrelationID: "other"}, {Seq: 5, Subject: "workboard.owned", Kind: event.KindWorkboardTaskUpdated, Payload: json.RawMessage(`broken`)}}
	s := appwork.NewWatch(store, watchJournal{rows: rows, cause: errors.New("legacy partial Range")})
	out, err := s.Watch(context.Background(), appwork.WatchInput{ID: "owned", RunID: "run", Limit: 3})
	if err != nil || out.Count != 3 || len(out.Events) != 3 || out.Events[0].Seq != 3 || out.Events[1].Seq != 4 || out.Events[2].Seq != 5 || len(out.BlockedDependencies) != 2 || out.BlockedDependencies[0].CreatedMS != 100 || out.BlockedDependencies[1].CreatedMS != 0 {
		t.Fatalf("snapshot=%+v err=%v", out, err)
	}
	raw, err := json.Marshal(out.Events)
	if err != nil {
		t.Fatal(err)
	}
	var views []map[string]any
	if err := json.Unmarshal(raw, &views); err != nil {
		t.Fatal(err)
	}
	if p, ok := views[0]["payload"]; !ok || p != nil {
		t.Fatalf("null payload=%s", raw)
	}
	if p, ok := views[1]["payload"].(map[string]any); !ok || len(p) != 0 {
		t.Fatalf("empty object payload=%s", raw)
	}
	if _, ok := views[2]["payload"]; ok {
		t.Fatalf("malformed payload not omitted=%s", raw)
	}
	if corr, ok := views[2]["correlation_id"]; !ok || corr != "" {
		t.Fatalf("required empty correlation=%s", raw)
	}
	all, err := s.Watch(context.Background(), appwork.WatchInput{ID: "owned", RunID: "run", Limit: 0})
	if err != nil || all.Count != 4 || all.Events[0].Seq != 1 {
		t.Fatalf("unbounded rows=%+v err=%v", all, err)
	}
}
