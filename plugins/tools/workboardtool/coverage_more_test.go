// SPDX-License-Identifier: MIT

package workboardtool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/workboard"
)

type covKernel struct {
	gotCreate workboard.CreateSpec
	gotClaim  string
	workboard *workboard.Store
}

func (f *covKernel) Workboard() *workboard.Store { return f.workboard }
func (f *covKernel) CreateWorkboardTask(_ string, spec workboard.CreateSpec) (workboard.Task, bool, error) {
	f.gotCreate = spec
	return workboard.Task{ID: "t-1", Title: spec.Title, Status: spec.Status, Owner: spec.Owner}, true, nil
}
func (f *covKernel) ClaimWorkboardTask(_, id, _, _ string) (workboard.Task, error) {
	f.gotClaim = id
	return workboard.Task{ID: id, Status: workboard.StatusRunning}, nil
}
func (f *covKernel) HeartbeatWorkboardTask(_, id, _, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id, Status: workboard.StatusRunning}, nil
}
func (f *covKernel) CommentWorkboardTask(_, id, _, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id}, nil
}
func (f *covKernel) BlockWorkboardTask(_, id, _, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id, Status: workboard.StatusBlocked}, nil
}
func (f *covKernel) FailWorkboardTask(_, id, _, _ string) (workboard.Task, workboard.RetryDecision, error) {
	return workboard.Task{ID: id, Status: workboard.StatusReview}, workboard.RetryDecision{Action: "block"}, nil
}
func (f *covKernel) UnblockWorkboardTask(_, id, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id, Status: workboard.StatusReady}, nil
}
func (f *covKernel) CompleteWorkboardTask(_, id, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id, Status: workboard.StatusDone}, nil
}
func (f *covKernel) ArchiveWorkboardTask(_, id, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id, Status: workboard.StatusArchived}, nil
}
func (f *covKernel) LinkWorkboardTask(_, id, _, _ string) (workboard.Task, error) {
	return workboard.Task{ID: id}, nil
}
func (f *covKernel) SetWorkboardRetryPolicy(_, id, _ string, p *workboard.RetryPolicy) (workboard.Task, error) {
	return workboard.Task{ID: id, RetryPolicy: p}, nil
}
func (f *covKernel) AddWorkboardDependency(_, id, dep string) (workboard.Task, error) {
	return workboard.Task{ID: id, Dependencies: []workboard.Dependency{{ID: dep}}}, nil
}
func (f *covKernel) ReclaimStaleWorkboardTask(_, id, _ string, _ time.Duration) (workboard.Task, error) {
	return workboard.Task{ID: id, Status: workboard.StatusReady}, nil
}

func TestWorkboardCoverageBindCurrent(t *testing.T) {
	tool := New()
	if tool.current() != nil {
		t.Fatal("New should leave kernel nil")
	}
	tool.Bind(&covKernel{})
	if tool.current() == nil {
		t.Fatal("Bind should set kernel")
	}
}

func TestWorkboardCoverageInvokeValidation(t *testing.T) {
	_, err := New().Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("unavailable = %+v", res)
	}

	tool := New()
	tool.Bind(&covKernel{})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("nil workboard = %+v", res)
	}

	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("workboard.OpenStore: %v", err)
	}
	tool = New()
	tool.Bind(&covKernel{workboard: st})
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"show"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "needs \"id\"") {
		t.Fatalf("show no id = %+v", res)
	}

	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"op":"show","id":"missing"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "no workboard task") {
		t.Fatalf("show missing = %+v", res)
	}
}

func TestWorkboardCoverageApplyContextDefaults(t *testing.T) {
	in := input{Op: "create", Title: "x"}
	in.applyContextDefaults("alice", "run-1")
	if in.Owner != "alice" {
		t.Fatalf("owner = %q", in.Owner)
	}
	if in.Agent != "alice" {
		t.Fatalf("agent = %q", in.Agent)
	}
	if in.RunID != "run-1" {
		t.Fatalf("run id = %q", in.RunID)
	}

	in = input{Owner: "bob", Agent: "carol", RunID: "run-2"}
	in.applyContextDefaults("alice", "run-1")
	if in.Owner != "bob" || in.Agent != "carol" || in.RunID != "run-2" {
		t.Fatalf("pre-filled overwritten: %+v", in)
	}
}

func TestWorkboardCoverageRetryPolicyFromInput(t *testing.T) {
	if got := retryPolicyFromInput(input{}); got != nil {
		t.Fatalf("empty input = %+v, want nil", got)
	}
	if got := retryPolicyFromInput(input{MaxAttempts: 3}); got == nil || got.MaxAttempts != 3 {
		t.Fatalf("attempts only = %+v", got)
	}
	if got := retryPolicyFromInput(input{EscalateTo: "boss"}); got == nil || got.EscalateTo != "boss" {
		t.Fatalf("escalate only = %+v", got)
	}
}

func TestWorkboardCoverageTaskOrError(t *testing.T) {
	r, err := taskOrError("ok", workboard.Task{ID: "t", Status: workboard.StatusDone}, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.IsError || !strings.Contains(r.Output, `"action": "ok"`) {
		t.Fatalf("success = %+v", r)
	}
	r, _ = taskOrError("ok", workboard.Task{}, errors.New("disk full"))
	if !r.IsError || !strings.Contains(r.Output, "disk full") {
		t.Fatalf("error = %+v", r)
	}
}

func TestWorkboardCoverageTaskDecisionOrError(t *testing.T) {
	r, err := taskDecisionOrError("failed", workboard.Task{ID: "t"}, workboard.RetryDecision{Action: "block"}, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if r.IsError || !strings.Contains(r.Output, `"decision"`) {
		t.Fatalf("success = %+v", r)
	}
	r, _ = taskDecisionOrError("failed", workboard.Task{}, workboard.RetryDecision{}, errors.New("nope"))
	if !r.IsError || !strings.Contains(r.Output, "nope") {
		t.Fatalf("error = %+v", r)
	}
}

func TestWorkboardCoverageOkJSONMarshalFail(t *testing.T) {
	r := okJSON(make(chan int))
	if !r.IsError || !strings.Contains(r.Output, "marshal:") {
		t.Fatalf("marshal fail = %+v", r)
	}
}

func TestWorkboardCoverageInvokeOps(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("workboard.OpenStore: %v", err)
	}
	tool := New()
	tool.Bind(&covKernel{workboard: st})

	cases := map[string]bool{
		`{"op":"list"}`: false,
		`{"op":"create","title":"t","description":"d","priority":3,"tags":["x"],"artifacts":["a"],"max_attempts":2,"escalate_to":"boss"}`: false,
		`{"op":"claim","id":"t-1"}`:                                 false,
		`{"op":"heartbeat","id":"t-1"}`:                             false,
		`{"op":"comment","id":"t-1","body":"note"}`:                 false,
		`{"op":"block","id":"t-1","reason":"need input"}`:           false,
		`{"op":"fail","id":"t-1","reason":"retry"}`:                 false,
		`{"op":"unblock","id":"t-1"}`:                               false,
		`{"op":"complete","id":"t-1"}`:                              false,
		`{"op":"archive","id":"t-1"}`:                               false,
		`{"op":"link","id":"t-1","type":"run","target":"r-1"}`:      false,
		`{"op":"policy","id":"t-1","max_attempts":3,"clear":false}`: false,
		`{"op":"policy","id":"t-1","clear":true}`:                   false,
		`{"op":"depend","id":"t-1","depends_on":"t-0"}`:             false,
		`{"op":"reclaim","id":"t-1","stale_after_sec":60}`:          false,
		`{"op":"reclaim","id":"t-1"}`:                               false,
		`{"op":""}`:                                                 true,
		`{"op":"unknown"}`:                                          true,
	}
	for input, isErr := range cases {
		res, err := tool.Invoke(context.Background(), json.RawMessage(input))
		if err != nil {
			t.Fatalf("Invoke %q: %v", input, err)
		}
		if isErr != res.IsError {
			t.Errorf("Invoke %q: IsError = %v, want %v (output: %s)", input, res.IsError, isErr, res.Output)
		}
	}
}
