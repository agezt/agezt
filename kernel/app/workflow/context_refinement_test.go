// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/contract/opapi"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"testing"
	"time"
)

func TestWorkflowCallerCancellationBlocksAllNineEffects(t *testing.T) {
	for _, mode := range []string{"save", "restore", "remove", "enable", "draft", "refine", "run", "node", "webhook"} {
		t.Run(mode, func(t *testing.T) {
			caller, cancel := context.WithCancel(opapi.WithCorrelation(context.Background(), "owned-op"))
			cancel()
			writer := &lifecyclePort{}
			designer := &designerPort{}
			calls, identities, jobs := 0, 0, 0
			correlation := func() string { identities++; return "fresh" }
			reader := &graphPort{items: []graphs.Workflow{webhookGraph("owned", true, false)}}
			copilot := NewCopilot(reader, designer, correlation)
			exec := NewExecution(reader, ExecutionHost{Correlation: correlation, Detach: func(func()) { jobs++ }, Run: func(_ context.Context, _ string, _ string, _ any) (RunResult, error) {
				calls++
				return RunResult{}, nil
			}, TestNode: func(_ context.Context, _ string, _ graphs.Workflow, _ string, _ map[string]any, _ any) (NodeResult, error) {
				calls++
				return NodeResult{}, nil
			}})
			var err error
			switch mode {
			case "save":
				_, err = NewLifecycle(writer).Save(caller, SaveInput{})
			case "restore":
				_, err = NewLifecycle(writer).Restore(caller, RestoreInput{})
			case "remove":
				_, err = NewLifecycle(writer).Remove(caller, RemoveInput{})
			case "enable":
				_, err = NewLifecycle(writer).SetEnabled(caller, EnableInput{})
			case "draft":
				_, err = copilot.Draft(caller, DraftInput{})
			case "refine":
				posted := graphs.Workflow{}
				plan, prepErr := copilot.PrepareRefine(RefineInput{Posted: &posted})
				if prepErr != nil {
					t.Fatal(prepErr)
				}
				_, err = plan.Refine(caller)
			case "run":
				_, err = exec.Run(caller, RunInput{Ref: "owned", Async: true})
			case "node":
				_, err = exec.TestNode(caller, NodeInput{})
			case "webhook":
				_, err = exec.Webhook(caller, WebhookInput{Ref: "owned", Secret: "owned fixture credential"})
			}
			effects := len(writer.Calls) + len(designer.calls) + calls + jobs
			if !errors.Is(err, context.Canceled) || effects != 0 || identities != 0 {
				t.Fatalf("EXPECTED: canceled caller blocks %s before effects/identity; ACTUAL: err=%v effects=%d identities=%d", mode, err, effects, identities)
			}
		})
	}
}
func TestWorkflowOwnedCorrelationAndCallerBudgetReachAllBlockingPorts(t *testing.T) {
	parent, cancel := context.WithTimeout(context.WithValue(context.Background(), callerValueKey{}, "caller"), 10*time.Second)
	defer cancel()
	ctx := opapi.WithCorrelation(parent, "owned-op")
	fallbacks := 0
	fallback := func() string { fallbacks++; return "wrong-fresh" }
	check := func(callCtx context.Context, corr string) {
		t.Helper()
		deadline, ok := callCtx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 8*time.Second || remaining > 11*time.Second || callCtx.Value(callerValueKey{}) != "caller" || corr != "owned-op" {
			t.Fatalf("EXPECTED: owned identity and caller budget/values; ACTUAL: corr=%s deadline=%v remaining=%v value=%v", corr, ok, remaining, callCtx.Value(callerValueKey{}))
		}
	}
	designer := &designerPort{check: func(call designCall) { check(call.Ctx, call.Corr) }}
	reader := &graphPort{items: []graphs.Workflow{webhookGraph("owned", true, true)}}
	copilot := NewCopilot(reader, designer, fallback)
	if out, err := copilot.Draft(ctx, DraftInput{}); err != nil || out.CorrelationID != "owned-op" {
		t.Fatal(out, err)
	}
	posted := graphs.Workflow{}
	plan, err := copilot.PrepareRefine(RefineInput{Posted: &posted})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := plan.Refine(ctx); err != nil || out.CorrelationID != "owned-op" {
		t.Fatal(out, err)
	}
	exec := NewExecution(reader, ExecutionHost{Correlation: fallback, Run: func(callCtx context.Context, corr, ref string, payload any) (RunResult, error) {
		check(callCtx, corr)
		return RunResult{}, nil
	}, TestNode: func(callCtx context.Context, corr string, w graphs.Workflow, node string, data map[string]any, payload any) (NodeResult, error) {
		check(callCtx, corr)
		return NodeResult{}, nil
	}})
	if out, err := exec.Run(ctx, RunInput{}); err != nil || out.CorrelationID != "owned-op" {
		t.Fatal(out, err)
	}
	if out, err := exec.TestNode(ctx, NodeInput{}); err != nil || out.CorrelationID != "owned-op" {
		t.Fatal(out, err)
	}
	if out, err := exec.Webhook(ctx, WebhookInput{Ref: "owned", Secret: "owned fixture credential"}); err != nil || out.CorrelationID != "owned-op" {
		t.Fatal(out, err)
	}
	writer := &lifecyclePort{}
	life := NewLifecycle(writer)
	if _, err := life.Save(ctx, SaveInput{CorrelationID: "untrusted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Restore(ctx, RestoreInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := life.SetEnabled(ctx, EnableInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := life.Remove(ctx, RemoveInput{}); err != nil {
		t.Fatal(err)
	}
	for _, call := range writer.Calls {
		if call.Corr != "owned-op" {
			t.Fatal(call)
		}
	}
	if fallbacks != 0 {
		t.Fatal("owned context allocated fallback", fallbacks)
	}
}
func TestWorkflowDetachedRetainsCallerValuesAndIdentityWithoutCallerDeadline(t *testing.T) {
	for _, mode := range []string{"manual", "webhook"} {
		t.Run(mode, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.WithValue(context.Background(), callerValueKey{}, "caller"), 10*time.Second)
			ctx := opapi.WithCorrelation(parent, "owned-op")
			defer cancel()
			reader := &graphPort{items: []graphs.Workflow{webhookGraph("owned", true, false)}}
			jobs := []func(){}
			runs := 0
			s := NewExecution(reader, ExecutionHost{Correlation: func() string { t.Fatal("owned detached used fresh fallback"); return "wrong" }, Detach: func(fn func()) { jobs = append(jobs, fn) }, Run: func(callCtx context.Context, corr, ref string, payload any) (RunResult, error) {
				runs++
				budget(t, callCtx, 15*time.Minute)
				if callCtx.Err() != nil || callCtx.Value(callerValueKey{}) != "caller" || corr != "owned-op" {
					t.Fatal(callCtx.Err(), callCtx.Value(callerValueKey{}), corr)
				}
				return RunResult{}, nil
			}})
			if mode == "manual" {
				out, err := s.Run(ctx, RunInput{Ref: "owned", Async: true})
				if err != nil || out.CorrelationID != "owned-op" {
					t.Fatal(out, err)
				}
			} else {
				out, err := s.Webhook(ctx, WebhookInput{Ref: "owned", Secret: "owned fixture credential"})
				if err != nil || out.CorrelationID != "owned-op" {
					t.Fatal(out, err)
				}
			}
			cancel()
			if len(jobs) != 1 {
				t.Fatal(len(jobs))
			}
			jobs[0]()
			if runs != 1 {
				t.Fatal(runs)
			}
		})
	}
}
func TestWorkflowInflightCancellationReachesDesignerAndRunPorts(t *testing.T) {
	for _, mode := range []string{"draft", "refine", "run", "node", "webhook"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(opapi.WithCorrelation(context.Background(), "owned-op"))
			entered := make(chan struct{})
			done := make(chan error, 1)
			wait := func(callCtx context.Context) error {
				close(entered)
				select {
				case <-callCtx.Done():
					return callCtx.Err()
				case <-time.After(2 * time.Second):
					return errors.New("caller cancellation did not reach port")
				}
			}
			designer := &blockingDesigner{wait: wait}
			reader := &graphPort{items: []graphs.Workflow{webhookGraph("owned", true, true)}}
			copilot := NewCopilot(reader, designer, func() string { return "fresh" })
			s := NewExecution(reader, ExecutionHost{Correlation: func() string { return "fresh" }, Run: func(callCtx context.Context, _ string, _ string, _ any) (RunResult, error) {
				return RunResult{}, wait(callCtx)
			}, TestNode: func(callCtx context.Context, _ string, _ graphs.Workflow, _ string, _ map[string]any, _ any) (NodeResult, error) {
				return NodeResult{}, wait(callCtx)
			}})
			go func() {
				var err error
				switch mode {
				case "draft":
					_, err = copilot.Draft(ctx, DraftInput{})
				case "refine":
					posted := graphs.Workflow{}
					plan, prepErr := copilot.PrepareRefine(RefineInput{Posted: &posted})
					if prepErr != nil {
						done <- prepErr
						return
					}
					_, err = plan.Refine(ctx)
				case "run":
					_, err = s.Run(ctx, RunInput{})
				case "node":
					_, err = s.TestNode(ctx, NodeInput{})
				case "webhook":
					_, err = s.Webhook(ctx, WebhookInput{Ref: "owned", Secret: "owned fixture credential"})
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				cancel()
				t.Fatal("port not entered")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("inflight cancellation stalled")
			}
		})
	}
}

type blockingDesigner struct{ wait func(context.Context) error }

func (p *blockingDesigner) DraftWorkflow(ctx context.Context, _ string, _ string, _ string) (graphs.Workflow, error) {
	return graphs.Workflow{}, p.wait(ctx)
}
func (p *blockingDesigner) RefineWorkflow(ctx context.Context, _ string, _ graphs.Workflow, _ string) (graphs.Workflow, error) {
	return graphs.Workflow{}, p.wait(ctx)
}
