// SPDX-License-Identifier: MIT

package workboardtool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/workboard"
)

func TestToolCreateClaimCommentComplete(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fh := &fakeKernel{st: st}
	tool := New()
	tool.Bind(fh)

	ctx := agent.WithAgent(agent.WithCorrelation(context.Background(), "run-123"), "builder")
	res, err := tool.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Ship workboard","assignee":"builder","idempotency_key":"wb-1"}`))
	if err != nil || res.IsError {
		t.Fatalf("create res=%+v err=%v", res, err)
	}
	var created struct {
		Created bool `json:"created"`
		Task    struct {
			ID    string `json:"id"`
			Owner string `json:"owner"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(res.Output), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, res.Output)
	}
	if !created.Created || created.Task.ID == "" || created.Task.Owner != "builder" {
		t.Fatalf("created = %+v", created)
	}

	for _, raw := range []string{
		`{"op":"claim","id":"` + created.Task.ID + `"}`,
		`{"op":"heartbeat","id":"` + created.Task.ID + `"}`,
		`{"op":"comment","id":"` + created.Task.ID + `","body":"ready for review"}`,
		`{"op":"complete","id":"` + created.Task.ID + `"}`,
	} {
		res, err = tool.Invoke(ctx, json.RawMessage(raw))
		if err != nil || res.IsError {
			t.Fatalf("%s res=%+v err=%v", raw, res, err)
		}
	}
	task, found := st.Get(created.Task.ID)
	if !found {
		t.Fatal("task not found")
	}
	if task.Status != workboard.StatusDone || len(task.Comments) == 0 || task.Comments[0].Author != "builder" {
		t.Fatalf("task = %+v", task)
	}
	if len(task.Attempts) != 1 || task.Attempts[0].RunID != "run-123" {
		t.Fatalf("attempts = %+v", task.Attempts)
	}
}

func TestToolRetryPolicyFail(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fh := &fakeKernel{st: st}
	tool := New()
	tool.Bind(fh)

	ctx := agent.WithAgent(agent.WithCorrelation(context.Background(), "run-123"), "builder")
	res, err := tool.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Retry","assignee":"builder","max_attempts":2,"escalate_to":"lead"}`))
	if err != nil || res.IsError {
		t.Fatalf("create res=%+v err=%v", res, err)
	}
	var created struct {
		Task struct {
			ID          string         `json:"id"`
			RetryPolicy map[string]any `json:"retry_policy"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(res.Output), &created); err != nil {
		t.Fatalf("decode create: %v\n%s", err, res.Output)
	}
	if created.Task.ID == "" || int(created.Task.RetryPolicy["max_attempts"].(float64)) != 2 {
		t.Fatalf("created = %+v", created)
	}
	if res, err = tool.Invoke(ctx, json.RawMessage(`{"op":"claim","id":"`+created.Task.ID+`"}`)); err != nil || res.IsError {
		t.Fatalf("claim res=%+v err=%v", res, err)
	}
	res, err = tool.Invoke(ctx, json.RawMessage(`{"op":"fail","id":"`+created.Task.ID+`","reason":"timeout"}`))
	if err != nil || res.IsError {
		t.Fatalf("fail res=%+v err=%v", res, err)
	}
	var failed struct {
		Decision struct {
			Retry       bool `json:"retry"`
			NextAttempt int  `json:"next_attempt"`
		} `json:"decision"`
		Task struct {
			Status string `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(res.Output), &failed); err != nil {
		t.Fatalf("decode fail: %v\n%s", err, res.Output)
	}
	if failed.Task.Status != "ready" || !failed.Decision.Retry || failed.Decision.NextAttempt != 2 {
		t.Fatalf("failed = %+v", failed)
	}
}

type fakeKernel struct {
	st *workboard.Store
}

func (f *fakeKernel) Workboard() *workboard.Store { return f.st }

func (f *fakeKernel) CreateWorkboardTask(_ string, spec workboard.CreateSpec) (workboard.Task, bool, error) {
	return f.st.Create(spec, time.Now())
}

func (f *fakeKernel) ClaimWorkboardTask(_ string, id, agent, runID string) (workboard.Task, error) {
	return f.st.Claim(id, agent, runID, time.Now())
}

func (f *fakeKernel) HeartbeatWorkboardTask(_ string, id, agent, runID string) (workboard.Task, error) {
	return f.st.Heartbeat(id, agent, runID, time.Now())
}

func (f *fakeKernel) CommentWorkboardTask(_ string, id, author, body string) (workboard.Task, error) {
	return f.st.Comment(id, author, body, time.Now())
}

func (f *fakeKernel) BlockWorkboardTask(_ string, id, actor, reason string) (workboard.Task, error) {
	return f.st.Block(id, actor, reason, time.Now())
}

func (f *fakeKernel) FailWorkboardTask(_ string, id, actor, reason string) (workboard.Task, workboard.RetryDecision, error) {
	return f.st.Fail(id, actor, reason, time.Now())
}

func (f *fakeKernel) UnblockWorkboardTask(_ string, id, actor string) (workboard.Task, error) {
	return f.st.Unblock(id, actor, time.Now())
}

func (f *fakeKernel) CompleteWorkboardTask(_ string, id, actor string) (workboard.Task, error) {
	return f.st.Complete(id, actor, time.Now())
}

func (f *fakeKernel) ArchiveWorkboardTask(_ string, id, actor string) (workboard.Task, error) {
	return f.st.Archive(id, actor, time.Now())
}

func (f *fakeKernel) LinkWorkboardTask(_ string, id, typ, target string) (workboard.Task, error) {
	return f.st.Link(id, typ, target, time.Now())
}

func (f *fakeKernel) SetWorkboardRetryPolicy(_ string, id, actor string, policy *workboard.RetryPolicy) (workboard.Task, error) {
	return f.st.SetRetryPolicy(id, actor, policy, time.Now())
}

func (f *fakeKernel) AddWorkboardDependency(_ string, id, dependsOn string) (workboard.Task, error) {
	return f.st.AddDependency(id, dependsOn, time.Now())
}

func (f *fakeKernel) ReclaimStaleWorkboardTask(_ string, id, actor string, staleAfter time.Duration) (workboard.Task, error) {
	return f.st.ReclaimStale(id, actor, staleAfter, time.Now())
}
