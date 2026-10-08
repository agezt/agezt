// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
)

type taskUpdateCalls struct{ refs []string }

// taskUpdateFixture runs the service over a real roster store, mapping misses
// the way the kernel's UpdateProfile does.
func taskUpdateFixture(t *testing.T, tasks ...core.AgentTask) (*TaskUpdateService, *core.Store, *taskUpdateCalls) {
	t.Helper()
	st, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Add(core.Profile{Slug: "ops", Soul: "s", TaskList: tasks}); err != nil {
		t.Fatal(err)
	}
	calls := &taskUpdateCalls{}
	return NewTaskUpdate(func(ref string, mutate func(*core.Profile)) (core.Profile, bool, error) {
		calls.refs = append(calls.refs, ref)
		p, err := st.Update(ref, mutate)
		if errors.Is(err, core.ErrNotFound) {
			return core.Profile{}, false, nil
		}
		return p, err == nil, err
	}), st, calls
}

func taskUpdateRun(t *testing.T, s *TaskUpdateService, raw string) (TaskUpdateOutput, error) {
	t.Helper()
	var in TaskUpdateRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.TaskUpdate(context.Background(), in)
}

func TestRosterTaskUpdateRejectsBeforeWriting(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`:                        "args.ref required",
		`{"ref":" ","op":"bogus"}`:  "args.ref required",
		`{"ref":3}`:                 "args.ref must be a string",
		`{"ref":null}`:              "args.ref must be a string",
		`{"ref":"ops","op":1}`:      "args.op must be a string",
		`{"ref":"ops","op":null}`:   "args.op must be a string",
		`{"ref":"ops","op":"move"}`: "args.op must be add, update, or remove",
		`{"ref":"ops","id":"a","task":["title"]}`:                      "args.task: json: cannot unmarshal array into Go value of type roster.AgentTask",
		`{"ref":"ops","op":"move","task":"x"}`:                         "args.op must be add, update, or remove",
		`{"ref":"ops","task":"x","title":3}`:                           "args.task: json: cannot unmarshal string into Go value of type roster.AgentTask",
		`{"ref":"ops","task":{"created_ms":"x"}}`:                      "args.task: json: cannot unmarshal string into Go struct field AgentTask.created_ms of type int64",
		`{"ref":"ops","id":4,"title":3}`:                               "args.id must be a string",
		`{"ref":"ops","title":3}`:                                      "args.title must be a string",
		`{"ref":"ops","description":null}`:                             "args.description must be a string",
		`{"ref":"ops","scope":[]}`:                                     "args.scope must be a string",
		`{"ref":"ops","status":{}}`:                                    "args.status must be a string",
		`{"ref":"ops","op":"add"}`:                                     "args.title required",
		`{"ref":"ops","op":"add","title":"  ","scope":"x"}`:            "args.title required",
		`{"ref":"ops","op":"ADD","task":{"title":"t"},"title":" "}`:    "args.title required",
		`{"ref":"ops","id":"a","title":""}`:                            "args.title required",
		`{"ref":"ops","id":"a","task":{"title":" "}}`:                  "args.title required",
		`{"ref":"ops","id":"a","scope":"forever","status":"x"}`:        "args.scope must be cycle or total",
		`{"ref":"ops","id":"a","task":{"scope":"forever"}}`:            "args.scope must be cycle or total",
		`{"ref":"ops","id":"a","task":{"scope":"cycle"},"scope":"no"}`: "args.scope must be cycle or total",
		`{"ref":"ops","id":"a","status":"later"}`:                      "args.status must be todo, doing, done, blocked, or retired",
		`{"ref":"ops","id":"a","task":{"status":"later"}}`:             "args.status must be todo, doing, done, blocked, or retired",
	} {
		s, _, calls := taskUpdateFixture(t)
		if _, err := taskUpdateRun(t, s, raw); err == nil || err.Error() != want || calls.refs != nil {
			t.Fatal(raw, err, calls.refs)
		}
	}
	// Validation only covers provided fields; flat values trim for the check.
	accepted := []string{}
	for _, scope := range []string{"cycle", "total", ""} {
		for _, status := range []string{"todo", "doing", "done", "blocked", "retired", ""} {
			accepted = append(accepted, `{"ref":"ops","id":"a","scope":"`+scope+`","status":"`+status+`"}`, `{"ref":"ops","id":"a","task":{"scope":"`+scope+`","status":"`+status+`"}}`)
		}
	}
	for _, raw := range append(accepted, `{"ref":"ops","op":"remove","id":"a","title":""}`, `{"ref":"ops","id":"a","scope":" cycle ","status":" done "}`, `{"ref":"ops","id":"a","task":{"scope":"","status":""}}`, `{"ref":"ops","id":"a","task":null}`) {
		s, _, calls := taskUpdateFixture(t)
		if _, err := taskUpdateRun(t, s, raw); err == nil || err.Error() != "unknown agent task: a" || len(calls.refs) != 1 {
			t.Fatal(raw, err, calls.refs)
		}
	}
}

func TestRosterTaskUpdateErrorsAfterTheProfileWrite(t *testing.T) {
	for raw, want := range map[string]string{
		`{"ref":"ghost","id":"a"}`:                          "unknown agent: ghost",
		`{"ref":"ops"}`:                                     "args.id required",
		`{"ref":"ops","op":"delete","id":"  "}`:             "args.id required",
		`{"ref":"ops","op":" Remove ","task":{"id":" x "}}`: "unknown agent task:  x ",
		`{"ref":"ops","id":"x","status":"done"}`:            "unknown agent task: x",
	} {
		s, st, calls := taskUpdateFixture(t)
		before, _ := st.Get("ops")
		if _, err := taskUpdateRun(t, s, raw); err == nil || err.Error() != want || len(calls.refs) != 1 {
			t.Fatal(raw, err, calls.refs)
		}
		// A missed task still goes through the store's journaled update.
		if after, _ := st.Get("ops"); !strings.HasPrefix(raw, `{"ref":"ghost"`) && after.UpdatedMS < before.UpdatedMS {
			t.Fatal("profile update skipped", raw)
		}
	}
	s, _, _ := taskUpdateFixture(t)
	failing := NewTaskUpdate(func(string, func(*core.Profile)) (core.Profile, bool, error) {
		return core.Profile{}, true, errors.New("disk full")
	})
	if _, err := taskUpdateRun(t, failing, `{"ref":"ops","op":"add","title":"t"}`); err == nil || err.Error() != "disk full" {
		t.Fatal(err)
	}
	_ = s
}

func TestRosterTaskUpdateAddUpdateRemove(t *testing.T) {
	s, st, _ := taskUpdateFixture(t)
	added, err := taskUpdateRun(t, s, `{"ref":"ops","op":" ADD ","title":" check queue ","description":" d ","scope":"cycle","task":{"title":"ignored","status":"doing"}}`)
	if err != nil || !added.Updated || added.Task.ID == "" || added.Task.Title != "check queue" || added.Task.Description != "d" || added.Task.Scope != "cycle" || added.Task.Status != "doing" || added.Task.CreatedMS == 0 || added.Task.UpdatedMS == 0 || len(added.Profile.TaskList) != 1 || added.Profile.TaskList[0] != added.Task {
		t.Fatal("add reports the stored task", added, err)
	}
	if added.Profile.Slug != "ops" || added.Profile.Kind != "custom" || added.Profile.Managed {
		t.Fatal("legacy profile view", added.Profile)
	}
	raw, _ := json.Marshal(added.Task)
	if !strings.HasPrefix(string(raw), `{"id":`) || !strings.Contains(string(raw), `"title":"check queue","description":"d","scope":"cycle","status":"doing","created_ms":`) {
		t.Fatal("task keeps struct member order", string(raw))
	}
	explicit, err := taskUpdateRun(t, s, `{"ref":"ops","op":"add","task":{"id":"t2","title":"check queue"}}`)
	if err != nil || explicit.Task.ID != "t2" || explicit.Task.Status != "todo" || explicit.Task.Scope != "total" {
		t.Fatal("explicit add id", explicit, err)
	}
	// Same title, no id: the last stored match is reported.
	again, err := taskUpdateRun(t, s, `{"ref":"ops","op":"add","title":"check queue"}`)
	if err != nil || again.Task.ID == added.Task.ID || again.Task.ID == "t2" || len(again.Profile.TaskList) != 3 || again.Task != again.Profile.TaskList[2] {
		t.Fatal("add resolves the newest same-title task", again, err)
	}

	id := added.Task.ID
	updated, err := taskUpdateRun(t, s, `{"ref":"ops","id":" `+id+` ","status":"done"}`)
	if err != nil || updated.Task.ID != id || updated.Task.Status != "done" || updated.Task.Title != "check queue" || updated.Task.Description != "d" || updated.Task.Scope != "cycle" || updated.Task.UpdatedMS != again.Profile.TaskList[0].UpdatedMS {
		t.Fatal("update reports the pre-normalized task", updated, err)
	}
	if p, _ := st.Get("ops"); p.TaskList[0].Status != "done" || p.TaskList[0].UpdatedMS < updated.Task.UpdatedMS {
		t.Fatal("update stored", p.TaskList)
	}
	// Flat presence clears; an empty args.task field does not.
	kept, _ := taskUpdateRun(t, s, `{"ref":"ops","op":"update","task":{"id":"`+id+`","description":"","scope":"","status":""}}`)
	if kept.Task.Description != "d" || kept.Task.Scope != "cycle" || kept.Task.Status != "done" {
		t.Fatal("empty task fields leave values", kept.Task)
	}
	cleared, _ := taskUpdateRun(t, s, `{"ref":"ops","id":"`+id+`","description":"","scope":"","status":""}`)
	if cleared.Task.Description != "" || cleared.Task.Scope != "" || cleared.Task.Status != "" {
		t.Fatal("flat presence clears", cleared.Task)
	}
	if p, _ := st.Get("ops"); p.TaskList[0].Scope != "total" || p.TaskList[0].Status != "todo" {
		t.Fatal("store normalizes cleared fields", p.TaskList[0])
	}
	retitled, _ := taskUpdateRun(t, s, `{"ref":"ops","task":{"id":"t2","title":"new"},"description":"x"}`)
	if retitled.Task.Title != "new" || retitled.Task.Description != "x" {
		t.Fatal("task title and flat description", retitled.Task)
	}

	removed, err := taskUpdateRun(t, s, `{"ref":"ops","op":"Delete","id":"t2"}`)
	if err != nil || removed.Task.ID != "t2" || removed.Task.Title != "new" || len(removed.Profile.TaskList) != 2 || removed.Profile.TaskList[0].ID != id || removed.Profile.TaskList[1].ID != again.Task.ID {
		t.Fatal("remove keeps neighbours in order", removed, err)
	}
	if _, err := taskUpdateRun(t, s, `{"ref":"ops","op":"remove","id":"t2"}`); err == nil || err.Error() != "unknown agent task: t2" {
		t.Fatal("removed twice", err)
	}
}

func TestRosterTaskUpdateOperationIsAudited(t *testing.T) {
	if _, err := TaskUpdateOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, _, calls := taskUpdateFixture(t)
	providers := 0
	ops, err := TaskUpdateOperations(func(ctx context.Context) *TaskUpdateService {
		providers++
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "agent_task_update" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[TaskUpdateRequest]() || spec.Output != reflect.TypeFor[TaskUpdateOutput]() || spec.HTTP != (opapi.HTTP{Method: "POST", Path: "/api/agents/task"}) {
		t.Fatal(spec)
	}
	profile := `{"id":"i","slug":"s","enabled":true,"created_ms":0,"updated_ms":0,"kind":"custom","managed":false}`
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"updated":true,"profile":`+profile+`,"task":{"title":"t"}}`)) != nil || schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"updated":true,"profile":`+profile+`,"task":{"title":"t","created_ms":"x"}}`)) == nil {
		t.Fatal("task schema")
	}
	audit := &recordingAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops","op":"add","title":"t"}`), nil); err == nil || err.Error() != "audit unavailable" || providers != 0 || calls.refs != nil {
		t.Fatal("failed audit admission must block the write", err, providers, calls.refs)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}, Audit: &recordingAudit{}})
		if _, err := denied.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops","op":"add","title":"t"}`), nil); err == nil || calls.refs != nil {
			t.Fatal("non-primary write", err)
		}
	}
	audit = &recordingAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}, Audit: audit})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops","op":"add","title":"t"}`), nil); !errors.Is(err, context.Canceled) || len(audit.begins) != 0 || calls.refs != nil {
		t.Fatal("canceled write", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops","op":"add","title":"t"}`), nil)
	if got, ok := out.(TaskUpdateOutput); err != nil || !ok || got.Task.Title != "t" || len(audit.begins) != 1 || audit.begins[0].Operation != "agent_task_update" || string(audit.begins[0].Input) != `{"ref":"ops","op":"add","title":"t"}` || len(audit.ends) != 1 || audit.ends[0] != nil {
		t.Fatal("audited success", out, err, audit)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops"}`), nil); err == nil || len(audit.ends) != 2 || audit.ends[1] == nil || audit.ends[1].Error() != "args.id required" {
		t.Fatal("audited failure", err, audit.ends)
	}
}
