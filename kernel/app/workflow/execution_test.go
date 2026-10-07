// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/runtime"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type executionWakeKey struct{}
type executionCall struct {
	Ctx       context.Context
	Corr, Ref string
	Payload   any
}

func budget(t *testing.T, ctx context.Context, want time.Duration) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	remaining := time.Until(deadline)
	if !ok || remaining < want-time.Second || remaining > want+time.Second || ctx.Err() != nil {
		t.Fatal(ok, remaining, ctx.Err())
	}
}
func TestWorkflowExecutionManualSyncAsyncPayloadBudgetAndErrors(t *testing.T) {
	reader := &graphPort{items: []graphs.Workflow{{Name: "canonical", ID: "owned-id"}}}
	corrs := 0
	calls := []executionCall{}
	jobs := []func(){}
	cause := error(nil)
	s := NewExecution(reader, ExecutionHost{Correlation: func() string { corrs++; return fmt.Sprintf("fresh-%d", corrs) }, WithWake: func(ctx context.Context, wake Wake) context.Context {
		return context.WithValue(ctx, executionWakeKey{}, wake)
	}, Detach: func(fn func()) { jobs = append(jobs, fn) }, Run: func(ctx context.Context, corr, ref string, payload any) (RunResult, error) {
		budget(t, ctx, 15*time.Minute)
		if ctx.Value(executionWakeKey{}) != (Wake{Source: "manual"}) {
			t.Fatal(ctx.Value(executionWakeKey{}))
		}
		calls = append(calls, executionCall{Ctx: ctx, Corr: corr, Ref: ref, Payload: payload})
		return RunResult{}, cause
	}})
	caller, cancel := context.WithCancel(context.Background())
	defer cancel()
	payload := []any{false, float64(0), map[string]any{"raw": true}}
	out, err := s.Run(caller, RunInput{Ref: " raw ref ", Payload: payload})
	if err != nil || out.CorrelationID != "fresh-1" || len(reader.refs) != 0 || len(calls) != 1 || calls[0].Ref != " raw ref " || !reflect.DeepEqual(calls[0].Payload, payload) {
		t.Fatal(out, err, reader.refs, calls)
	}
	wire := object(t, out)
	if !reflect.DeepEqual(wire, map[string]any{"correlation_id": "fresh-1", "executed": nil, "outputs": nil}) {
		t.Fatal(wire)
	}
	select {
	case <-calls[0].Ctx.Done():
	default:
		t.Fatal("sync context retained")
	}
	out, err = s.Run(caller, RunInput{Ref: " owned-id ", Payload: payload, Async: true})
	if err != nil || !reflect.DeepEqual(object(t, out), map[string]any{"correlation_id": "fresh-2", "accepted": true, "async": true, "workflow": "canonical"}) || !reflect.DeepEqual(reader.refs, []string{"owned-id"}) || len(jobs) != 1 || len(calls) != 1 {
		t.Fatal(out, err, reader.refs, len(jobs), calls)
	}
	reader.items[0].Name = "changed"
	jobs[0]()
	if calls[1].Ref != "canonical" || calls[1].Corr != "fresh-2" || !reflect.DeepEqual(calls[1].Payload, payload) {
		t.Fatal(calls)
	}
	select {
	case <-calls[1].Ctx.Done():
	default:
		t.Fatal("detached context retained")
	}
	missing, err := s.Run(caller, RunInput{Ref: " missing ", Async: true})
	if !errors.Is(err, graphs.ErrNotFound) || err.Error() != "unknown workflow:  missing " || !reflect.DeepEqual(missing, RunOutput{}) || corrs != 3 || len(jobs) != 1 {
		t.Fatal(missing, err, corrs, jobs)
	}
	cause = errors.New("owned runner cause")
	out, err = s.Run(caller, RunInput{Ref: "raw"})
	if !errors.Is(err, cause) || err.Error() != "owned runner cause (correlation fresh-4)" || !reflect.DeepEqual(out, RunOutput{}) {
		t.Fatal(out, err)
	}
	cause = graphs.ErrNotFound
	out, err = s.Run(caller, RunInput{Ref: " missing "})
	if !errors.Is(err, graphs.ErrNotFound) || err.Error() != "unknown workflow:  missing " || !reflect.DeepEqual(out, RunOutput{}) {
		t.Fatal(out, err)
	}
}
func TestWorkflowExecutionNodeProbeRetainsRawArgumentsZeroFieldsAndCause(t *testing.T) {
	graph := graphs.Workflow{Name: "posted"}
	data := map[string]any{"owned": map[string]any{"output": false}}
	payload := []any{true}
	cause := error(nil)
	var callCtx context.Context
	corrs := 0
	s := NewExecution(&graphPort{}, ExecutionHost{Correlation: func() string { corrs++; return "node-corr" }, WithWake: func(context.Context, Wake) context.Context { t.Fatal("probe acquired run wake"); return nil }, TestNode: func(ctx context.Context, corr string, w graphs.Workflow, node string, upstream map[string]any, payloadValue any) (NodeResult, error) {
		budget(t, ctx, 3*time.Minute)
		if corr != "node-corr" || !reflect.DeepEqual(w, graph) || node != " node " || !reflect.DeepEqual(upstream, data) || !reflect.DeepEqual(payloadValue, payload) {
			t.Fatal(corr, w, node, upstream, payloadValue)
		}
		callCtx = ctx
		return NodeResult{}, cause
	}})
	out, err := s.TestNode(context.Background(), NodeInput{Workflow: graph, Node: " node ", Data: data, Payload: payload})
	if err != nil || !reflect.DeepEqual(object(t, out), map[string]any{"output": nil, "port": "", "attempts": float64(0), "correlation_id": "node-corr"}) || corrs != 1 {
		t.Fatal(out, err, corrs)
	}
	select {
	case <-callCtx.Done():
	default:
		t.Fatal("node context retained")
	}
	cause = errors.New("owned node cause")
	out, err = s.TestNode(context.Background(), NodeInput{Workflow: graph, Node: " node ", Data: data, Payload: payload})
	if err != cause || !reflect.DeepEqual(out, NodeOutput{}) {
		t.Fatal(out, err)
	}
}
func webhookGraph(name string, enabled, reply bool) graphs.Workflow {
	raw, _ := json.Marshal(map[string]any{"kind": "webhook", "secret": "owned fixture credential", "reply": reply})
	return graphs.Workflow{Name: name, Enabled: enabled, Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger, Config: raw}}}
}
func TestWorkflowExecutionWebhookUniformGateAndReplyDetachedVariants(t *testing.T) {
	for _, mode := range []string{"invalid", "blank", "secret-empty", "missing", "paused", "wrong-kind", "wrong-secret", "reply", "detached"} {
		t.Run(mode, func(t *testing.T) {
			reader := &graphPort{items: []graphs.Workflow{webhookGraph("canonical", true, mode == "reply")}}
			corrs := 0
			calls := []executionCall{}
			jobs := []func(){}
			input := WebhookInput{Ref: " canonical ", Secret: "owned fixture credential", Payload: map[string]any{"raw": true}}
			switch mode {
			case "invalid":
				input.Invalid = true
			case "blank":
				input.Ref = " "
			case "secret-empty":
				input.Secret = ""
			case "missing":
				input.Ref = "missing"
			case "paused":
				reader.items[0].Enabled = false
			case "wrong-kind":
				reader.items[0].Nodes[0].Config = json.RawMessage(`{"kind":"manual","secret":"owned fixture credential"}`)
			case "wrong-secret":
				input.Secret = "wrong"
			}
			s := NewExecution(reader, ExecutionHost{Correlation: func() string { corrs++; return "webhook-corr" }, Detach: func(fn func()) { jobs = append(jobs, fn) }, WithWake: func(ctx context.Context, wake Wake) context.Context {
				return context.WithValue(ctx, executionWakeKey{}, wake)
			}, Run: func(ctx context.Context, corr, ref string, payload any) (RunResult, error) {
				want := 15 * time.Minute
				if mode == "reply" {
					want = 2 * time.Minute
				}
				budget(t, ctx, want)
				if ctx.Value(executionWakeKey{}) != (Wake{Source: "webhook", TriggerSubject: "webhook:canonical"}) || corr != "webhook-corr" || ref != "canonical" || !reflect.DeepEqual(payload, input.Payload) {
					t.Fatal(ctx.Value(executionWakeKey{}), corr, ref, payload)
				}
				calls = append(calls, executionCall{Ctx: ctx, Corr: corr, Ref: ref, Payload: payload})
				return RunResult{}, nil
			}})
			out, err := s.Webhook(context.Background(), input)
			if mode != "reply" && mode != "detached" {
				if err != ErrWebhookRefused || err.Error() != "webhook refused" || !reflect.DeepEqual(out, WebhookOutput{}) || corrs != 0 || len(calls) != 0 || len(jobs) != 0 {
					t.Fatal(out, err, corrs, calls, jobs)
				}
				if (mode == "invalid" || mode == "blank" || mode == "secret-empty") && len(reader.refs) != 0 {
					t.Fatal(reader.refs)
				}
				return
			}
			if err != nil || corrs != 1 || out.CorrelationID != "webhook-corr" || out.Workflow != "canonical" {
				t.Fatal(out, err, corrs)
			}
			wire := object(t, out)
			if mode == "reply" {
				if !reflect.DeepEqual(wire, map[string]any{"correlation_id": "webhook-corr", "workflow": "canonical", "executed": nil, "outputs": nil}) || len(calls) != 1 || len(jobs) != 0 {
					t.Fatal(wire, calls, jobs)
				}
			} else {
				if !reflect.DeepEqual(wire, map[string]any{"correlation_id": "webhook-corr", "workflow": "canonical", "accepted": true}) || len(calls) != 0 || len(jobs) != 1 {
					t.Fatal(wire, calls, jobs)
				}
				jobs[0]()
				if len(calls) != 1 {
					t.Fatal(calls)
				}
			}
			select {
			case <-calls[0].Ctx.Done():
			default:
				t.Fatal("webhook context retained")
			}
		})
	}
	cause := errors.New("owned webhook cause")
	s := NewExecution(&graphPort{items: []graphs.Workflow{webhookGraph("owned", true, true)}}, ExecutionHost{Correlation: func() string { return "corr" }, Run: func(context.Context, string, string, any) (RunResult, error) { return RunResult{}, cause }})
	out, err := s.Webhook(context.Background(), WebhookInput{Ref: "owned", Secret: "owned fixture credential"})
	if !errors.Is(err, cause) || err.Error() != "webhook run failed: owned webhook cause (correlation corr)" || !reflect.DeepEqual(out, WebhookOutput{}) {
		t.Fatal(out, err)
	}
}
func TestWorkflowExecutionDetachedPanicRetainsWakeAndCleanup(t *testing.T) {
	wake := Wake{Source: "webhook", Reason: "reason", ScheduleID: "schedule", StandingID: "standing", StandingName: "name", TriggerSubject: "trigger", ParentCorrelation: "parent"}
	var runCtx context.Context
	panics := 0
	s := NewExecution(&graphPort{}, ExecutionHost{WithWake: func(ctx context.Context, got Wake) context.Context {
		if got != wake {
			t.Fatal(got)
		}
		return ctx
	}, Run: func(ctx context.Context, corr, ref string, payload any) (RunResult, error) {
		budget(t, ctx, 15*time.Minute)
		runCtx = ctx
		if corr != "corr" || ref != "owned" || payload != false {
			t.Fatal(corr, ref, payload)
		}
		panic("owned panic")
	}, Panic: func(got Wake, corr, name string, value any) {
		panics++
		if got != wake || corr != "corr" || name != "owned" || value != "owned panic" {
			t.Fatal(got, corr, name, value)
		}
		select {
		case <-runCtx.Done():
		default:
			t.Fatal("panic retained run context")
		}
	}})
	s.detached(context.Background(), wake, "corr", "owned", false)
	if panics != 1 {
		t.Fatal(panics)
	}
	done := make(chan string, 1)
	s = NewExecution(&graphPort{items: []graphs.Workflow{{Name: "owned"}}}, ExecutionHost{Correlation: func() string { return "default-detach" }, Run: func(context.Context, string, string, any) (RunResult, error) {
		done <- "ran"
		return RunResult{}, errors.New("ignored detached failure")
	}})
	out, err := s.Run(context.Background(), RunInput{Ref: "owned", Async: true})
	if err != nil || out.CorrelationID != "default-detach" {
		t.Fatal(out, err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("default goroutine did not run")
	}
}
func TestWorkflowExecutionUsesOwnedActualRuntimeTransformAndProbe(t *testing.T) {
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	graph, _, err := k.SaveWorkflow("", graphs.Workflow{Name: "owned", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}, {ID: "format", Type: graphs.NodeTransform, Config: json.RawMessage(`{"template":"{{trigger.payload}}"}`)}}, Edges: []graphs.Edge{{From: "start", To: "format"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := NewExecution(k.Workflows(), ExecutionHost{Correlation: k.NewCorrelation, Run: func(ctx context.Context, corr, ref string, payload any) (RunResult, error) {
		res, err := k.RunWorkflow(ctx, corr, ref, payload)
		return RunResult{Executed: res.Executed, Outputs: res.Outputs}, err
	}, TestNode: func(ctx context.Context, corr string, w graphs.Workflow, node string, data map[string]any, payload any) (NodeResult, error) {
		res, err := k.TestWorkflowNode(ctx, corr, w, node, data, payload)
		return NodeResult{Output: res.Output, Port: res.Port, Attempts: res.Attempts}, err
	}})
	out, err := s.Run(context.Background(), RunInput{Ref: "owned", Payload: "owned payload"})
	if err != nil || out.CorrelationID == "" || out.Executed == nil || !reflect.DeepEqual(*out.Executed, []string{"start", "format"}) || out.Outputs == nil || (*out.Outputs)["format"] != "owned payload" {
		t.Fatal(out, err)
	}
	node, err := s.TestNode(context.Background(), NodeInput{Workflow: graph, Node: "format", Payload: "probe payload"})
	if err != nil || node.Output != "probe payload" || node.Attempts != 1 || node.CorrelationID == "" || node.CorrelationID == out.CorrelationID || provider.CallCount() != 0 {
		t.Fatal(node, err, provider.CallCount())
	}
}
