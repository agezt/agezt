// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeSteerer is a test double for agent.Steerer. Wait honours a pause gate;
// Drain hands back queued directives once.
type fakeSteerer struct {
	mu         sync.Mutex
	directives []agent.Directive
	gate       chan struct{} // non-nil + open ⇒ Wait blocks until closed/ctx-done
}

func (f *fakeSteerer) Wait(ctx context.Context) error {
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func (f *fakeSteerer) Drain() []agent.Directive {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.directives
	f.directives = nil
	return out
}

// steerNoopTool is a trivial tool so the loop's first response can request a
// tool call and proceed to a second iteration (where steering is observed).
type steerNoopTool struct{}

func (steerNoopTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "noop", Description: "does nothing", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (steerNoopTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	return agent.Result{Output: "ok"}, nil
}

// TestSteer_DirectiveFoldedAsUserTurn: a directive queued before the run is
// drained and folded into the conversation as a user turn (with the operator
// prefix), and a run.steered event carrying it is journaled.
func TestSteer_DirectiveFoldedAsUserTurn(t *testing.T) {
	b, j := newTestBus(t)
	// First call asks for the noop tool (→ a second iteration); second call ends.
	prov := mock.New(testToolUse("t1", "noop", map[string]any{}), mock.FinalText("done"))
	var sawDirective bool
	var mu sync.Mutex
	prov.OnRequest = func(req agent.CompletionRequest) {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "[operator steering] focus on the database") {
				mu.Lock()
				sawDirective = true
				mu.Unlock()
			}
		}
	}
	st := &fakeSteerer{directives: []agent.Directive{{Text: "focus on the database"}}}

	answer, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "agent-steer",
		CorrelationID: "corr-steer",
		Model:         "test",
		MaxIter:       5,
		Tools:         map[string]agent.Tool{"noop": steerNoopTool{}},
		Steer:         st,
	}, "do the thing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if answer != "done" {
		t.Errorf("answer = %q want done", answer)
	}
	mu.Lock()
	if !sawDirective {
		t.Error("the steering directive was never folded into a request's messages")
	}
	mu.Unlock()

	// A run.steered event carrying the directive must be journaled.
	var directive string
	var seen bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindRunSteered {
			seen = true
			var p struct {
				Directive string `json:"directive"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			directive = p.Directive
		}
		return nil
	})
	if !seen {
		t.Fatal("no run.steered event journaled")
	}
	if directive != "focus on the database" {
		t.Errorf("run.steered directive = %q want 'focus on the database'", directive)
	}
}

// TestSteer_NoteUsesSofterPrefix (M962): a Note directive ("BTW") is folded with
// the note prefix (not the steer prefix) and the run.steered event records
// mode=note, so the model treats it as FYI rather than a re-prioritisation.
func TestSteer_NoteUsesSofterPrefix(t *testing.T) {
	b, j := newTestBus(t)
	prov := mock.New(testToolUse("t1", "noop", map[string]any{}), mock.FinalText("done"))
	var sawNote, sawSteerPrefix bool
	var mu sync.Mutex
	prov.OnRequest = func(req agent.CompletionRequest) {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, "operator note") && strings.Contains(m.Content, "check the cache too") {
				mu.Lock()
				sawNote = true
				mu.Unlock()
			}
			if strings.Contains(m.Content, "[operator steering]") {
				mu.Lock()
				sawSteerPrefix = true
				mu.Unlock()
			}
		}
	}
	st := &fakeSteerer{directives: []agent.Directive{{Text: "check the cache too", Note: true}}}
	if _, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "a", CorrelationID: "corr-note", Model: "test", MaxIter: 5,
		Tools: map[string]agent.Tool{"noop": steerNoopTool{}}, Steer: st,
	}, "do the thing"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawNote {
		t.Error("note directive was not folded with the operator-note prefix")
	}
	if sawSteerPrefix {
		t.Error("a note must NOT use the forceful [operator steering] prefix")
	}
	var mode string
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindRunSteered {
			var p struct {
				Mode string `json:"mode"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			mode = p.Mode
		}
		return nil
	})
	if mode != "note" {
		t.Errorf("run.steered mode = %q want note", mode)
	}
}

// TestSteer_PauseHonoursCancel: a paused run that is cancelled returns an error
// rather than hanging, and never reaches the provider — pause must never make a
// run un-killable.
func TestSteer_PauseHonoursCancel(t *testing.T) {
	b, _ := newTestBus(t)
	prov := mock.New(mock.FinalText("unreached"))
	st := &fakeSteerer{gate: make(chan struct{})} // open, never closed ⇒ blocks until ctx-done

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled: the very first Wait must observe it

	_, err := agent.Run(ctx, agent.LoopConfig{
		Provider:      prov,
		Bus:           b,
		Actor:         "agent-steer",
		CorrelationID: "corr-steer-cancel",
		Model:         "test",
		MaxIter:       5,
		Steer:         st,
	}, "do the thing")
	if err == nil {
		t.Fatal("expected a context error from a cancelled paused run, got nil")
	}
	if prov.CallCount() != 0 {
		t.Errorf("provider must not be called while paused-then-cancelled; calls=%d", prov.CallCount())
	}
}
