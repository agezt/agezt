// SPDX-License-Identifier: MIT

package workboard_test

import (
	"context"
	"errors"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type lifecycleHost struct {
	cause  error
	method string
	args   []string
	spec   tasks.CreateSpec
	ctx    context.Context
	now    time.Time
}

func (h *lifecycleHost) result(method string, args ...string) (tasks.Task, error) {
	h.method = method
	h.args = args
	return tasks.Task{ID: "owned", Title: "fixture", Status: tasks.StatusTriage, Comments: []tasks.Comment{{Body: "body"}}}, h.cause
}
func (h *lifecycleHost) CreateWorkboardTask(corr string, spec tasks.CreateSpec) (tasks.Task, bool, error) {
	h.spec = spec
	t, err := h.result("create", corr)
	return t, true, err
}
func (h *lifecycleHost) ClaimWorkboardTask(corr, id, agent, run string) (tasks.Task, error) {
	return h.result("claim", corr, id, agent, run)
}
func (h *lifecycleHost) HeartbeatWorkboardTask(corr, id, agent, run string) (tasks.Task, error) {
	return h.result("heartbeat", corr, id, agent, run)
}
func (h *lifecycleHost) CommentWorkboardTask(corr, id, author, body string) (tasks.Task, error) {
	return h.result("comment", corr, id, author, body)
}
func (h *lifecycleHost) BlockWorkboardTask(corr, id, actor, reason string) (tasks.Task, error) {
	return h.result("block", corr, id, actor, reason)
}
func (h *lifecycleHost) FailWorkboardTask(corr, id, actor, reason string) (tasks.Task, tasks.RetryDecision, error) {
	task, err := h.result("fail", corr, id, actor, reason)
	return task, tasks.RetryDecision{Action: "retry", FailureCount: 1, Retry: true, MaxAttempts: 3, NextAttempt: 2, EscalateTo: "lead", Reason: reason}, err
}
func (h *lifecycleHost) UnblockWorkboardTask(corr, id, actor string) (tasks.Task, error) {
	return h.result("unblock", corr, id, actor)
}
func (h *lifecycleHost) CompleteWorkboardTask(corr, id, actor string) (tasks.Task, error) {
	return h.result("complete", corr, id, actor)
}
func (h *lifecycleHost) ProveTask(ctx context.Context, corr, id, answer string) (tasks.Task, error) {
	h.ctx = ctx
	return h.result("prove", corr, id, answer)
}
func (h *lifecycleHost) ArchiveWorkboardTask(corr, id, actor string) (tasks.Task, error) {
	return h.result("archive", corr, id, actor)
}
func (h *lifecycleHost) SetSeat(id, seat string, now time.Time) (tasks.Task, error) {
	h.now = now
	return h.result("seat", id, seat)
}
func TestWorkboardLifecycleRetainsAllFacadeInputsAndOriginalCauses(t *testing.T) {
	host := &lifecycleHost{}
	service := appwork.NewLifecycle(host, host)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	spec := tasks.CreateSpec{Title: "fixture", Description: "description", Tenant: "acme", Assignee: "writer", AcceptanceCriteria: []string{"one"}, RetryPolicy: &tasks.RetryPolicy{MaxAttempts: 3}}
	claim := appwork.ClaimInput{CorrelationID: "corr", ID: "id", Agent: "agent", RunID: "run"}
	reason := appwork.ReasonInput{CorrelationID: "corr", ID: "id", Actor: "actor", Reason: "reason"}
	calls := []struct {
		name string
		args []string
		run  func() error
	}{{"create", []string{"corr"}, func() error {
		out, err := service.Create(ctx, appwork.CreateInput{CorrelationID: "corr", Spec: spec})
		if err == nil && (!out.Created || out.Task.ID != "owned" || out.Task.CommentCount != 1) {
			t.Fatal("create output changed")
		}
		return err
	}}, {"claim", []string{"corr", "id", "agent", "run"}, func() error { _, err := service.Claim(ctx, claim); return err }}, {"heartbeat", []string{"corr", "id", "agent", "run"}, func() error { _, err := service.Heartbeat(ctx, claim); return err }}, {"comment", []string{"corr", "id", "author", "body"}, func() error {
		_, err := service.Comment(ctx, appwork.CommentInput{CorrelationID: "corr", ID: "id", Author: "author", Body: "body"})
		return err
	}}, {"block", []string{"corr", "id", "actor", "reason"}, func() error { _, err := service.Block(ctx, reason); return err }}, {"fail", []string{"corr", "id", "actor", "reason"}, func() error {
		out, err := service.Fail(ctx, reason)
		if err == nil && (out.Task.ID != "owned" || out.Decision.Action != "retry" || out.Decision.FailureCount != 1 || !out.Decision.Retry || out.Decision.Exhausted || out.Decision.MaxAttempts != 3 || out.Decision.NextAttempt != 2 || out.Decision.EscalateTo != "lead" || out.Decision.Reason != "reason") {
			t.Fatalf("retry output=%+v", out)
		}
		return err
	}}, {"unblock", []string{"corr", "id", "actor"}, func() error { _, err := service.Unblock(ctx, reason); return err }}, {"complete", []string{"corr", "id", "actor"}, func() error { _, err := service.Complete(ctx, reason); return err }}, {"prove", []string{"corr", "id", "answer"}, func() error {
		_, err := service.Prove(ctx, appwork.ProveInput{CorrelationID: "corr", ID: "id", Answer: "answer"})
		return err
	}}, {"seat", []string{"id", "local"}, func() error { _, err := service.Seat(ctx, appwork.SeatInput{ID: "id", Seat: "local"}); return err }}, {"archive", []string{"corr", "id", "actor"}, func() error { _, err := service.Archive(ctx, reason); return err }}}
	cause := errors.New("owned facade failure")
	for _, tc := range calls {
		*host = lifecycleHost{}
		before := time.Now()
		if err := tc.run(); err != nil || host.method != tc.name || !reflect.DeepEqual(host.args, tc.args) {
			t.Fatalf("%s args=%v method=%s err=%v", tc.name, host.args, host.method, err)
		}
		if tc.name == "create" && !reflect.DeepEqual(host.spec, spec) {
			t.Fatal("create spec changed")
		}
		if tc.name == "prove" && host.ctx != ctx {
			t.Fatal("prove caller context changed")
		}
		if tc.name == "seat" && (host.now.Before(before) || host.now.After(time.Now())) {
			t.Fatal("seat clock changed")
		}
		host.cause = cause
		if err := tc.run(); err != cause {
			t.Fatalf("%s cause=%v", tc.name, err)
		}
	}
}
func TestWorkboardLifecycleRetainsRealKernelTransitionsAndCorrelation(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	service := appwork.NewLifecycle(k, k.Workboard())
	ctx := context.Background()
	created, err := service.Create(ctx, appwork.CreateInput{CorrelationID: "owned-flow", Spec: tasks.CreateSpec{Title: "fixture", IdempotencyKey: "idempotent", RetryPolicy: &tasks.RetryPolicy{MaxAttempts: 2}}})
	if err != nil || !created.Created {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	again, err := service.Create(ctx, appwork.CreateInput{CorrelationID: "owned-flow", Spec: tasks.CreateSpec{Title: "fixture", IdempotencyKey: "idempotent"}})
	if err != nil || again.Created || again.Task.ID != created.Task.ID {
		t.Fatalf("idempotence=%+v err=%v", again, err)
	}
	id := created.Task.ID
	claim := appwork.ClaimInput{CorrelationID: "owned-flow", ID: id, Agent: "writer", RunID: "run"}
	claimed, err := service.Claim(ctx, claim)
	if err != nil || claimed.Task.Status != tasks.StatusRunning || claimed.Task.Claim == nil || claimed.Task.Claim.Agent != "writer" {
		t.Fatalf("claim=%+v err=%v", claimed, err)
	}
	if _, err := service.Heartbeat(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Comment(ctx, appwork.CommentInput{CorrelationID: "owned-flow", ID: id, Author: "writer", Body: "owned comment"}); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Fail(ctx, appwork.ReasonInput{CorrelationID: "owned-flow", ID: id, Actor: "writer", Reason: "owned failure"})
	if err != nil || !failed.Decision.Retry || failed.Decision.NextAttempt != 2 || failed.Task.FailedAttemptCount != 1 {
		t.Fatalf("fail=%+v err=%v", failed, err)
	}
	if _, err := service.Seat(ctx, appwork.SeatInput{ID: id, Seat: "local"}); err != nil {
		t.Fatal(err)
	}
	archived, err := service.Archive(ctx, appwork.ReasonInput{CorrelationID: "owned-flow", ID: id, Actor: "writer"})
	if err != nil || archived.Task.Status != tasks.StatusArchived {
		t.Fatalf("archive=%+v err=%v", archived, err)
	}
	seen := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == "owned-flow" {
			seen++
		}
		return nil
	}); err != nil || seen < 5 {
		t.Fatalf("kernel event corr count=%d err=%v", seen, err)
	}
}
