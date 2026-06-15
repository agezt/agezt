// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// steerLoopProvider keeps the loop iterating (returning a tool call) until it
// sees an operator-steering directive in the conversation, then ends the run
// with a sentinel answer. This lets a test drive a live, long-running agent and
// observe pause/inject/resume end-to-end.
type steerLoopProvider struct{}

func (steerLoopProvider) Name() string { return "steer-loop" }

func (steerLoopProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "[operator steering] finish now") {
			return &agent.CompletionResponse{
				Message:    agent.Message{Role: agent.RoleAssistant, Content: "steered-done"},
				StopReason: agent.StopEndTurn,
			}, nil
		}
	}
	return &agent.CompletionResponse{
		Message: agent.Message{
			Role:      agent.RoleAssistant,
			ToolCalls: []agent.ToolCall{{ID: "t", Name: "tick", Input: json.RawMessage(`{}`)}},
		},
		StopReason: agent.StopToolUse,
	}, nil
}

// tickTool is a harmless slow tool: the small sleep keeps each loop iteration
// from completing instantly, so the test can pause the run before it exhausts
// MaxIter.
type tickTool struct{}

func (tickTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "tick", Description: "noop tick", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (tickTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	time.Sleep(10 * time.Millisecond)
	return agent.Result{Output: "tick"}, nil
}

// TestSteerRun_PauseInjectResume drives a live run: pause it, confirm it reports
// paused, inject a directive that ends it, resume, and verify it finishes with
// the steered answer — with run.paused / run.resumed / run.steered all journaled.
func TestSteerRun_PauseInjectResume(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: steerLoopProvider{},
		Tools:    map[string]agent.Tool{"tick": tickTool{}},
		MaxIter:  500, // generous headroom so timing can't exhaust the loop
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	corr := k.NewCorrelation()
	done := make(chan struct {
		ans string
		err error
	}, 1)
	go func() {
		ans, e := k.RunWith(context.Background(), corr, "loop until steered")
		done <- struct {
			ans string
			err error
		}{ans, e}
	}()

	// Let the run get going, then pause it.
	time.Sleep(60 * time.Millisecond)
	if !k.PauseRun(corr) {
		t.Fatal("PauseRun returned false for a live run")
	}
	if paused, _, ok := k.RunControlState(corr); !ok || !paused {
		t.Fatalf("RunControlState = (paused=%v ok=%v) want paused & ok", paused, ok)
	}

	// Inject the directive that ends the run, then resume so the loop folds it.
	if !k.SteerRun(corr, "finish now", false) {
		t.Fatal("SteerRun returned false for a live run")
	}
	if !k.ResumeRun(corr) {
		t.Fatal("ResumeRun returned false for a paused run")
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("run errored: %v", r.err)
		}
		if r.ans != "steered-done" {
			t.Errorf("answer = %q want steered-done (directive not folded?)", r.ans)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("steered run did not finish")
	}

	// The control + steer events must be on the run's timeline.
	kinds := map[event.Kind]bool{}
	steered := ""
	var mu sync.Mutex
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != corr {
			return nil
		}
		mu.Lock()
		kinds[e.Kind] = true
		if e.Kind == event.KindRunSteered {
			var p struct {
				Directive string `json:"directive"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			steered = p.Directive
		}
		mu.Unlock()
		return nil
	})
	for _, want := range []event.Kind{event.KindRunPaused, event.KindRunResumed, event.KindRunSteered} {
		if !kinds[want] {
			t.Errorf("missing %s event on the run timeline", want)
		}
	}
	if steered != "finish now" {
		t.Errorf("run.steered directive = %q want 'finish now'", steered)
	}
}

// subDelegateSteerProvider drives a two-run conversation: the LEAD delegates
// once; the CHILD loops on the tick tool until it sees an operator-steering
// directive, then ends. Roles are told apart by the first user message — the
// child's delegated task is prefixed CHILD-TASK.
type subDelegateSteerProvider struct{}

func (subDelegateSteerProvider) Name() string { return "sub-steer" }

func (subDelegateSteerProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	var firstUser string
	for _, m := range req.Messages {
		if m.Role == agent.RoleUser {
			firstUser = m.Content
			break
		}
	}
	if strings.HasPrefix(strings.TrimSpace(firstUser), "CHILD-TASK") {
		// Child path: end once steered, else keep ticking.
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "[operator steering] finish now") {
				return &agent.CompletionResponse{
					Message:    agent.Message{Role: agent.RoleAssistant, Content: "child-steered-done"},
					StopReason: agent.StopEndTurn,
				}, nil
			}
		}
		return &agent.CompletionResponse{
			Message:    agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "t", Name: "tick", Input: json.RawMessage(`{}`)}}},
			StopReason: agent.StopToolUse,
		}, nil
	}
	// Lead path: delegate once, then finish after the child's answer returns.
	for _, m := range req.Messages {
		if m.Role == agent.RoleTool {
			return &agent.CompletionResponse{
				Message:    agent.Message{Role: agent.RoleAssistant, Content: "lead-done"},
				StopReason: agent.StopEndTurn,
			}, nil
		}
	}
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, ToolCalls: []agent.ToolCall{{ID: "d", Name: "delegate", Input: json.RawMessage(`{"task":"CHILD-TASK loop until steered"}`)}}},
		StopReason: agent.StopToolUse,
	}, nil
}

// TestSteerRun_SubAgentIndividuallySteerable proves an INDIVIDUAL sub-agent
// (not just the top-level lead) can be paused / steered / resumed from the
// cockpit (M631): the steering control is registered under the child's own
// correlation, so the per-run control API reaches into the delegation tree.
func TestSteerRun_SubAgentIndividuallySteerable(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:          t.TempDir(),
		Provider:         subDelegateSteerProvider{},
		Tools:            map[string]agent.Tool{"tick": tickTool{}},
		SubAgentTool:     true,
		SubAgentMaxDepth: 1,
		MaxIter:          500, // headroom so timing can't exhaust the child loop
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	col := &collector{}
	col.watch(k)

	leadCorr := k.NewCorrelation()
	done := make(chan struct {
		ans string
		err error
	}, 1)
	go func() {
		ans, e := k.RunWith(context.Background(), leadCorr, "lead-task: delegate then report")
		done <- struct {
			ans string
			err error
		}{ans, e}
	}()

	// Wait for the sub-agent to spawn and capture its correlation.
	childCorr := ""
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && childCorr == ""; {
		for _, e := range col.ofKind(event.KindSubAgentSpawned) {
			var p struct {
				Child string `json:"child_correlation"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.Child != "" {
				childCorr = p.Child
			}
		}
		if childCorr == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if childCorr == "" {
		t.Fatal("sub-agent never spawned")
	}
	if childCorr == leadCorr {
		t.Fatal("child correlation must differ from the lead")
	}

	// The SUB-AGENT — addressed by its own correlation — must be steerable.
	if !k.PauseRun(childCorr) {
		t.Fatal("PauseRun returned false for the live sub-agent (control not registered?)")
	}
	if paused, _, ok := k.RunControlState(childCorr); !ok || !paused {
		t.Fatalf("sub-agent RunControlState = (paused=%v ok=%v), want paused & ok", paused, ok)
	}
	if !k.SteerRun(childCorr, "finish now", false) {
		t.Fatal("SteerRun returned false for the live sub-agent")
	}
	if !k.ResumeRun(childCorr) {
		t.Fatal("ResumeRun returned false for the paused sub-agent")
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("run errored: %v", r.err)
		}
		if r.ans != "lead-done" {
			t.Errorf("answer = %q want lead-done", r.ans)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("steered sub-agent run did not finish")
	}

	// pause/steer/resume must be journaled under the CHILD correlation — proving
	// the control surface (and its audit) is the sub-agent's, not the lead's.
	kinds := map[event.Kind]bool{}
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == childCorr {
			kinds[e.Kind] = true
		}
		return nil
	})
	for _, want := range []event.Kind{event.KindRunPaused, event.KindRunSteered, event.KindRunResumed} {
		if !kinds[want] {
			t.Errorf("missing %s event on the SUB-AGENT timeline", want)
		}
	}
}

// TestSteerRun_UnknownReturnsFalse: steering a non-existent run is a no-op
// reporting false, never a panic.
func TestSteerRun_UnknownReturnsFalse(t *testing.T) {
	k := newKernel(t, steerLoopProvider{})
	if k.PauseRun("nope") || k.ResumeRun("nope") || k.StepRun("nope") || k.SteerRun("nope", "x", false) {
		t.Error("steering an unknown correlation must return false")
	}
	if _, _, ok := k.RunControlState("nope"); ok {
		t.Error("RunControlState for an unknown correlation must report ok=false")
	}
	if k.SteerRun("nope", "", false) {
		t.Error("an empty directive must be rejected")
	}
}
