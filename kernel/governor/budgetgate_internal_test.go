// SPDX-License-Identifier: MIT

package governor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// Every budget scope must journal which ceiling stopped the call. The three
// scopes used to build their own payloads, and had drifted: the daemon-wide one
// was the only one WITHOUT a "scope" field, so an operator or observer filtering
// budget.exceeded by scope saw task and agent breaches but never a global one —
// the most important of the three.
func TestBudgetScopes_AllJournalTheirScope(t *testing.T) {
	for _, tc := range []struct {
		scope string
		cfg   Config
		req   agent.CompletionRequest
		want  map[string]any // extra identifying fields beyond spent/ceiling/scope
	}{
		{
			scope: "global",
			cfg:   Config{DailyCeilingMicrocents: 100},
		},
		{
			scope: "task",
			cfg:   Config{TaskBudgets: map[string]int64{"research": 100}},
			req:   agent.CompletionRequest{TaskType: "research"},
			want:  map[string]any{"task_type": "research"},
		},
		{
			scope: "agent",
			cfg:   Config{},
			req:   agent.CompletionRequest{Agent: "scout", AgentDailyCeilingMc: 100},
			want:  map[string]any{"agent": "scout"},
		},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			b, j := busForTest(t)
			g := newBudgetGov(tc.cfg)
			g.spentByAgentToday = map[string]int64{}
			g.SetBus(b)

			// Put the scope's ledger at its ceiling.
			g.mu.Lock()
			g.spentToday.Store(100)
			g.spentByTaskToday["research"] = 100
			g.spentByAgentToday["scout"] = 100
			g.mu.Unlock()

			req := tc.req
			err := g.gateBudgets(&req)
			if err == nil {
				t.Fatalf("%s scope at its ceiling must refuse the call", tc.scope)
			}
			// The error names the scope's own sentinel so callers can tell the
			// ceilings apart.
			if !strings.Contains(err.Error(), "budget") {
				t.Errorf("refusal %q does not read as a budget refusal", err)
			}

			pl := lastPayload(t, j, event.KindBudgetExceeded)
			if got := pl["scope"]; got != tc.scope {
				t.Errorf(`payload["scope"] = %v, want %q`, got, tc.scope)
			}
			for k, want := range tc.want {
				if got := pl[k]; got != want {
					t.Errorf("payload[%q] = %v, want %v", k, got, want)
				}
			}
			for _, k := range []string{"spent_microcents", "ceiling_microcents"} {
				if _, ok := pl[k]; !ok {
					t.Errorf("payload is missing %q — the operator cannot see how far past the cap they are", k)
				}
			}
		})
	}
}

// An unconfigured ceiling is a no-op, not a refusal: a request with no task type
// or no named agent must not be stopped by caps that do not apply to it.
func TestGateBudgets_InapplicableScopesDoNotRefuse(t *testing.T) {
	g := newBudgetGov(Config{TaskBudgets: map[string]int64{"research": 1}})
	g.spentByAgentToday = map[string]int64{}
	g.mu.Lock()
	g.spentByTaskToday["research"] = 999 // another task type is over its cap
	g.mu.Unlock()

	req := agent.CompletionRequest{TaskType: "coding"} // no cap configured
	if err := g.gateBudgets(&req); err != nil {
		t.Errorf("a task type with no configured cap must pass: %v", err)
	}
	bare := agent.CompletionRequest{} // no task type, no agent, no global ceiling
	if err := g.gateBudgets(&bare); err != nil {
		t.Errorf("a request subject to no ceiling must pass: %v", err)
	}
}

// busForTest wires a real bus over a temp journal, so these assertions read the
// payload exactly as it lands on disk rather than as it was handed to Publish.
func busForTest(t *testing.T) (*bus.Bus, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	return bus.New(j), j
}

func lastPayload(t *testing.T, j *journal.Journal, kind event.Kind) map[string]any {
	t.Helper()
	var out map[string]any
	_ = j.Range(func(e *event.Event) error {
		if e.Kind != kind {
			return nil
		}
		var pl map[string]any
		if err := json.Unmarshal(e.Payload, &pl); err == nil {
			out = pl
		}
		return nil
	})
	if out == nil {
		t.Fatalf("no %s event was journaled", kind)
	}
	return out
}
