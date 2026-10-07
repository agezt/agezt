// SPDX-License-Identifier: MIT
package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	graphs "github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type designCall struct {
	Method, Corr, Name, Text string
	Base                     graphs.Workflow
	Ctx                      context.Context
}
type designerPort struct {
	calls  []designCall
	result graphs.Workflow
	cause  error
	check  func(designCall)
}

func (p *designerPort) DraftWorkflow(ctx context.Context, corr, name, description string) (graphs.Workflow, error) {
	call := designCall{Method: "draft", Corr: corr, Name: name, Text: description, Ctx: ctx}
	p.calls = append(p.calls, call)
	if p.check != nil {
		p.check(call)
	}
	return p.result, p.cause
}
func (p *designerPort) RefineWorkflow(ctx context.Context, corr string, base graphs.Workflow, instruction string) (graphs.Workflow, error) {
	call := designCall{Method: "refine", Corr: corr, Base: base, Text: instruction, Ctx: ctx}
	p.calls = append(p.calls, call)
	if p.check != nil {
		p.check(call)
	}
	return p.result, p.cause
}

type callerValueKey struct{}

func TestWorkflowCopilotFreshBudgetInputAndCleanup(t *testing.T) {
	graph := graphs.Workflow{Name: "designed", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}}}
	port := &designerPort{result: graph}
	corrs := 0
	order := []string{}
	port.check = func(call designCall) {
		order = append(order, call.Method)
		deadline, ok := call.Ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining < 179*time.Second || remaining > 181*time.Second || call.Ctx.Err() != nil || call.Ctx.Value(callerValueKey{}) != "caller" {
			t.Fatal(ok, remaining, call.Ctx.Err())
		}
	}
	s := NewCopilot(&graphPort{}, port, func() string { corrs++; order = append(order, "correlation"); return fmt.Sprintf("fresh-%d", corrs) })
	caller, cancel := context.WithCancel(context.WithValue(context.Background(), callerValueKey{}, "caller"))
	defer cancel()
	out, err := s.Draft(caller, DraftInput{Name: " raw name ", Description: " raw description "})
	if err != nil || out.CorrelationID != "fresh-1" || !reflect.DeepEqual(out.Workflow, ProjectFull(graph)) || port.calls[0].Name != " raw name " || port.calls[0].Text != " raw description " || port.calls[0].Corr != "fresh-1" || !reflect.DeepEqual(order, []string{"correlation", "draft"}) {
		t.Fatal(out, err, port.calls, order)
	}
	select {
	case <-port.calls[0].Ctx.Done():
	default:
		t.Fatal("draft context retained after return")
	}
	posted := graphs.Workflow{Name: "posted"}
	plan, err := s.PrepareRefine(RefineInput{Posted: &posted, Ref: "missing", Instruction: " raw instruction "})
	if err != nil || corrs != 1 {
		t.Fatal(plan, err, corrs)
	}
	out, err = plan.Refine(caller)
	if err != nil || out.CorrelationID != "fresh-2" || !reflect.DeepEqual(out.Workflow, ProjectFull(graph)) || port.calls[1].Base.Name != "posted" || port.calls[1].Text != " raw instruction " || port.calls[1].Corr != "fresh-2" || !reflect.DeepEqual(order, []string{"correlation", "draft", "correlation", "refine"}) {
		t.Fatal(out, err, port.calls, order)
	}
	select {
	case <-port.calls[1].Ctx.Done():
	default:
		t.Fatal("refine context retained after return")
	}
}
func TestWorkflowCopilotBasePrecedenceSnapshotAndNoPreflightEffects(t *testing.T) {
	reader := &graphPort{items: []graphs.Workflow{{Name: "stored", ID: "stored-id"}}}
	port := &designerPort{}
	corrs := 0
	s := NewCopilot(reader, port, func() string { corrs++; return "fresh" })
	posted := graphs.Workflow{}
	plan, err := s.PrepareRefine(RefineInput{Posted: &posted, Ref: " missing ", Instruction: "edit"})
	if err != nil || !reflect.DeepEqual(plan.base, posted) || len(reader.refs) != 0 || corrs != 0 || len(port.calls) != 0 {
		t.Fatal(plan, err, reader.refs, corrs, port.calls)
	}
	plan, err = s.PrepareRefine(RefineInput{Ref: " stored-id ", Instruction: "edit"})
	if err != nil || plan.base.Name != "stored" || !reflect.DeepEqual(reader.refs, []string{"stored-id"}) || corrs != 0 {
		t.Fatal(plan, err, reader.refs, corrs)
	}
	reader.items[0].Name = "changed"
	if _, err := plan.Refine(context.Background()); err != nil || port.calls[0].Base.Name != "stored" || corrs != 1 {
		t.Fatal(err, port.calls, corrs)
	}
	for _, ref := range []string{"", " ", " missing "} {
		before := corrs
		calls := len(port.calls)
		plan, err := s.PrepareRefine(RefineInput{Ref: ref})
		if err == nil || !reflect.DeepEqual(plan, Refinement{}) || corrs != before || len(port.calls) != calls {
			t.Fatal(ref, plan, err, corrs, port.calls)
		}
		if ref == " missing " {
			if !errors.Is(err, graphs.ErrNotFound) || err.Error() != "unknown workflow:  missing " {
				t.Fatal(err)
			}
		} else if err != ErrRefineBaseRequired || err.Error() != "args.workflow or args.ref required" {
			t.Fatal(err)
		}
	}
}
func TestWorkflowCopilotPreservesDesignerCauseAndZeroOutput(t *testing.T) {
	cause := errors.New("owned designer cause")
	port := &designerPort{cause: cause, result: graphs.Workflow{Name: "must not escape"}}
	s := NewCopilot(&graphPort{}, port, func() string { return "fresh" })
	out, err := s.Draft(context.Background(), DraftInput{})
	if err != cause || !reflect.DeepEqual(out, CopilotOutput{}) {
		t.Fatal(out, err)
	}
	posted := graphs.Workflow{}
	plan, err := s.PrepareRefine(RefineInput{Posted: &posted})
	if err != nil {
		t.Fatal(err)
	}
	out, err = plan.Refine(context.Background())
	if err != cause || !reflect.DeepEqual(out, CopilotOutput{}) {
		t.Fatal(out, err)
	}
	for _, call := range port.calls {
		select {
		case <-call.Ctx.Done():
		default:
			t.Fatal("failed call retained context", call)
		}
	}
}
func TestWorkflowCopilotUsesActualRuntimeMockAndReturnsUnsavedGraphs(t *testing.T) {
	raw := `{"name":"model-pick","description":"designed","nodes":[{"id":"start","type":"trigger"}]}`
	provider := mock.New(mock.FinalText(raw), mock.FinalText(raw))
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	base, _, err := k.SaveWorkflow("", graphs.Workflow{Name: "stored", Nodes: []graphs.Node{{ID: "start", Type: graphs.NodeTrigger}}})
	if err != nil {
		t.Fatal(err)
	}
	before := k.Workflows().List()
	s := NewCopilot(k.Workflows(), k, k.NewCorrelation)
	drafted, err := s.Draft(context.Background(), DraftInput{Name: "owned-draft", Description: "design owned graph"})
	if err != nil || drafted.CorrelationID == "" || drafted.Workflow.Name != "owned-draft" || drafted.Workflow.NodeCount != 1 {
		t.Fatal(drafted, err)
	}
	plan, err := s.PrepareRefine(RefineInput{Ref: base.ID, Instruction: "revise owned graph"})
	if err != nil {
		t.Fatal(err)
	}
	refined, err := plan.Refine(context.Background())
	if err != nil || refined.CorrelationID == "" || refined.CorrelationID == drafted.CorrelationID || refined.Workflow.Name != "stored" || provider.CallCount() != 2 || !reflect.DeepEqual(before, k.Workflows().List()) {
		t.Fatal(refined, err, provider.CallCount(), before, k.Workflows().List())
	}
	events := []*event.Event{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindWorkflowDrafted {
			events = append(events, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].CorrelationID != drafted.CorrelationID || events[1].CorrelationID != refined.CorrelationID {
		t.Fatal(events)
	}
	for i, e := range events {
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		want := []string{"draft", "refine"}[i]
		if payload["mode"] != want || e.Actor != "workflow" {
			t.Fatal(e, payload)
		}
	}
}
