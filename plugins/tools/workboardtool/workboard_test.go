// SPDX-License-Identifier: MIT

package workboardtool

import (
	"context"
	"encoding/json"
	"fmt"
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

// --- Pure helper function tests ---

func TestWorkboard_Definition(t *testing.T) {
	tl := New()
	def := tl.Definition()
	if def.Name != "workboard" {
		t.Errorf("Definition().Name = %q, want %q", def.Name, "workboard")
	}
	if def.Description == "" {
		t.Error("Definition().Description should not be empty")
	}
	if def.InputSchema == nil {
		t.Error("Definition().InputSchema should not be nil")
	}
}

func TestWorkboard_ErrResult(t *testing.T) {
	res := errResult("test error")
	if !res.IsError {
		t.Error("errResult should return IsError=true")
	}
	if res.Output != "workboard: test error" {
		t.Errorf("errResult output = %q", res.Output)
	}
}

func TestWorkboard_OkJSON_MarshalError(t *testing.T) {
	res := okJSON(make(chan int))
	if !res.IsError {
		t.Error("okJSON(channel) should return an error")
	}
}

func TestTaskView_OmitsEmptyOptionals(t *testing.T) {
	v := taskView(workboard.Task{ID: "t1", Title: "test", Status: "todo"})
	if v["description"] != nil {
		t.Error("taskView should omit description when empty")
	}
	if v["idempotency_key"] != nil {
		t.Error("taskView should omit idempotency_key when empty")
	}
	if v["tags"] != nil {
		t.Error("taskView should omit tags when empty")
	}
	if v["claim"] != nil {
		t.Error("taskView should omit claim when nil")
	}
	if v["completed_ms"] != nil {
		t.Error("taskView should omit completed_ms when 0")
	}
	if v["archived_ms"] != nil {
		t.Error("taskView should omit archived_ms when 0")
	}
}

func TestTaskView_IncludesNonZeroOptionals(t *testing.T) {
	v := taskView(workboard.Task{
		ID: "t1", Title: "test", Status: "todo",
		Description:    "details",
		IdempotencyKey: "ik-1",
		Tags:           []string{"urgent"},
		Artifacts:      []string{"log.txt"},
		RetryPolicy:    &workboard.RetryPolicy{MaxAttempts: 2},
		Dependencies:   []workboard.Dependency{{ID: "t0"}},
		Comments:       []workboard.Comment{{Body: "note"}},
		Links:          []workboard.Link{{Type: "run", Target: "r-1"}},
		BlockReason:    "blocked on t0",
		CompletedMS:    100,
		ArchivedMS:     200,
	})
	if v["description"].(string) != "details" {
		t.Error("taskView should include description")
	}
	if v["idempotency_key"].(string) != "ik-1" {
		t.Error("taskView should include idempotency_key")
	}
	if len(v["tags"].([]string)) != 1 {
		t.Error("taskView should include tags")
	}
	if len(v["artifacts"].([]string)) != 1 {
		t.Error("taskView should include artifacts")
	}
	if v["completed_ms"].(int64) != 100 {
		t.Error("taskView should include completed_ms")
	}
	if v["archived_ms"].(int64) != 200 {
		t.Error("taskView should include archived_ms")
	}
}

func TestTaskOrError_ErrorReturnsErrResult(t *testing.T) {
	res, err := taskOrError("action", workboard.Task{}, fmt.Errorf("kernel error"))
	if err != nil {
		t.Fatalf("taskOrError returned hard error: %v", err)
	}
	if !res.IsError {
		t.Error("taskOrError with error should return IsError=true")
	}
}

func TestTaskDecisionOrError_ErrorReturnsErrResult(t *testing.T) {
	res, err := taskDecisionOrError("fail", workboard.Task{}, workboard.RetryDecision{}, fmt.Errorf("kernel error"))
	if err != nil {
		t.Fatalf("taskDecisionOrError returned hard error: %v", err)
	}
	if !res.IsError {
		t.Error("taskDecisionOrError with error should return IsError=true")
	}
}

func TestWorkboard_ToolUnbound(t *testing.T) {
	tl := New()
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("unbound Invoke returned hard error: %v", err)
	}
	if !res.IsError {
		t.Fatal("unbound tool should return IsError=true")
	}
}

func TestWorkboard_ShowNotFound(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	res, err := tl.Invoke(context.Background(), json.RawMessage(`{"op":"show","id":"nonexistent"}`))
	if err != nil {
		t.Fatalf("show nonexistent returned hard error: %v", err)
	}
	if !res.IsError {
		t.Fatal("show nonexistent should return IsError=true")
	}
}

func TestWorkboard_ShowNeedsID(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	if res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"op":"show"}`)); !res.IsError {
		t.Error("show without id should error")
	}
}

func TestWorkboard_BlockAndUnblock(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	ctx := agent.WithAgent(context.Background(), "builder")
	res, _ := tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"block-test"}`))
	var created struct{ Task struct{ ID string } }
	json.Unmarshal([]byte(res.Output), &created)

	if res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"block","id":"`+created.Task.ID+`","reason":"waiting"}`)); res.IsError {
		t.Fatalf("block errored: %s", res.Output)
	}
	if res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"unblock","id":"`+created.Task.ID+`"}`)); res.IsError {
		t.Fatalf("unblock errored: %s", res.Output)
	}
}

func TestWorkboard_ArchiveAndLink(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	ctx := agent.WithAgent(context.Background(), "builder")
	// Create the dependency task first.
	res, _ := tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"prereq"}`))
	var dep struct{ Task struct{ ID string } }
	json.Unmarshal([]byte(res.Output), &dep)
	// Create the main task.
	res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"archive-test"}`))
	var created struct{ Task struct{ ID string } }
	json.Unmarshal([]byte(res.Output), &created)

	if res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"link","id":"`+created.Task.ID+`","type":"run","target":"r-123"}`)); res.IsError {
		t.Fatalf("link errored: %s", res.Output)
	}
	if res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"depend","id":"`+created.Task.ID+`","depends_on":"`+dep.Task.ID+`"}`)); res.IsError {
		t.Fatalf("depend errored: %s", res.Output)
	}
}

func TestWorkboard_EmptyOp(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	if res, _ := tl.Invoke(context.Background(), json.RawMessage(`{}`)); !res.IsError {
		t.Error("empty op should error")
	}
}

func TestWorkboard_UnknownOp(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	if res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"op":"frobnicate"}`)); !res.IsError {
		t.Error("unknown op should error")
	}
}

func TestWorkboard_List(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	ctx := agent.WithAgent(context.Background(), "builder")
	// Create two tasks.
	tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Task One","tenant":"team-a"}`))
	tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Task Two","tenant":"team-a"}`))
	// List all.
	res, _ := tl.Invoke(ctx, json.RawMessage(`{"op":"list"}`))
	var result struct {
		Count int              `json:"count"`
		Tasks []map[string]any `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(res.Output), &result); err != nil {
		t.Fatalf("list decode error: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("list count = %d, want 2", result.Count)
	}
}

func TestWorkboard_ListWithStatusFilter(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	ctx := agent.WithAgent(context.Background(), "builder")
	tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Task One","status":"triage"}`))
	tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Task Two","status":"ready"}`))
	res, _ := tl.Invoke(ctx, json.RawMessage(`{"op":"list","status":"triage"}`))
	var result struct {
		Count int `json:"count"`
	}
	json.Unmarshal([]byte(res.Output), &result)
	if result.Count != 1 {
		t.Errorf("filtered list count = %d, want 1", result.Count)
	}
}

func TestWorkboard_ListWithBadStatus(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	if res, _ := tl.Invoke(context.Background(), json.RawMessage(`{"op":"list","status":"bogus"}`)); !res.IsError {
		t.Error("list with bad status should error")
	}
}

func TestWorkboard_ListLimits(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	ctx := agent.WithAgent(context.Background(), "builder")
	for i := 0; i < 5; i++ {
		tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"Task"}`))
	}
	// Limit to 2.
	res, _ := tl.Invoke(ctx, json.RawMessage(`{"op":"list","limit":2}`))
	var result struct {
		Count int `json:"count"`
	}
	json.Unmarshal([]byte(res.Output), &result)
	if result.Count > 2 {
		t.Errorf("limited list count = %d, want <= 2", result.Count)
	}
}

func TestWorkboard_Policy(t *testing.T) {
	st, err := workboard.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tl := New()
	tl.Bind(&fakeKernel{st: st})
	ctx := agent.WithAgent(context.Background(), "builder")
	res, _ := tl.Invoke(ctx, json.RawMessage(`{"op":"create","title":"policy-test"}`))
	var created struct{ Task struct{ ID string } }
	json.Unmarshal([]byte(res.Output), &created)
	// Set policy.
	if res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"policy","id":"`+created.Task.ID+`","max_attempts":3,"escalate_to":"lead"}`)); res.IsError {
		t.Fatalf("policy errored: %s", res.Output)
	}
	// Clear policy.
	if res, _ = tl.Invoke(ctx, json.RawMessage(`{"op":"policy","id":"`+created.Task.ID+`","clear":true}`)); res.IsError {
		t.Fatalf("policy clear errored: %s", res.Output)
	}
}

var _ agent.Tool = New()
