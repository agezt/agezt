// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestDelegate_AsNamedAgent: delegate(agent="researcher") runs the sub-agent AS
// that roster profile — the provider sees the profile's model and its soul in
// the system prompt, and the spawn event records who the child ran as (M784).
func TestDelegate_AsNamedAgent(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "dig deep", "agent": "researcher"}),
		mock.FinalText("found it"), // child's run
		mock.FinalText("done"),     // lead's final
	)
	var reqs []agent.CompletionRequest
	prov.OnRequest = func(req agent.CompletionRequest) { reqs = append(reqs, req) }
	k := openSubAgentKernel(t, prov, 1)

	if _, err := k.AddProfile(roster.Profile{
		Slug: "researcher", Soul: "You are Researcher. Cite sources.", Model: "agent-model",
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	col := &collector{}
	col.watch(k)

	ans, _, err := k.Run(context.Background(), "lead task")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans != "done" {
		t.Errorf("final answer = %q", ans)
	}

	// The child's completion request carried the profile's model + soul.
	var child *agent.CompletionRequest
	for i := range reqs {
		if reqs[i].Model == "agent-model" {
			child = &reqs[i]
			break
		}
	}
	if child == nil {
		t.Fatalf("no provider request used the profile model; models seen: %v",
			func() []string {
				var m []string
				for _, r := range reqs {
					m = append(m, r.Model)
				}
				return m
			}())
	}
	if !strings.Contains(child.System, "You are Researcher. Cite sources.") {
		t.Errorf("child system prompt missing the soul: %q", child.System)
	}

	// The spawn event records the agent the child ran AS.
	time.Sleep(50 * time.Millisecond)
	spawns := col.ofKind(event.KindSubAgentSpawned)
	if len(spawns) != 1 {
		t.Fatalf("expected 1 spawn, got %d", len(spawns))
	}
	var pl struct {
		Agent string `json:"agent"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(spawns[0].Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Agent != "researcher" || pl.Model != "agent-model" {
		t.Errorf("spawn payload agent=%q model=%q, want researcher/agent-model", pl.Agent, pl.Model)
	}
}

// TestDelegate_ExplicitModelWinsOverProfile: an explicit delegate model beats
// the profile's — the profile only fills gaps.
func TestDelegate_ExplicitModelWinsOverProfile(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{
			"task": "t", "agent": "researcher", "model": "explicit-model"}),
		mock.FinalText("child"),
		mock.FinalText("lead"),
	)
	var models []string
	prov.OnRequest = func(req agent.CompletionRequest) { models = append(models, req.Model) }
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{Slug: "researcher", Model: "agent-model"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, m := range models {
		if m == "agent-model" {
			t.Fatalf("profile model used despite explicit override; models: %v", models)
		}
	}
	found := false
	for _, m := range models {
		if m == "explicit-model" {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit model never reached the provider; models: %v", models)
	}
}

// TestDelegate_UnknownOrPausedAgentRefused: a bad agent ref is a tool error the
// lead adapts to — no sub-agent is ever spawned.
func TestDelegate_UnknownOrPausedAgentRefused(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, k interface {
			AddProfile(roster.Profile) (roster.Profile, error)
			SetProfileEnabled(string, bool) (roster.Profile, error)
		})
	}{
		{"unknown agent", nil},
		{"paused agent", func(t *testing.T, k interface {
			AddProfile(roster.Profile) (roster.Profile, error)
			SetProfileEnabled(string, bool) (roster.Profile, error)
		}) {
			if _, err := k.AddProfile(roster.Profile{Slug: "ghost"}); err != nil {
				t.Fatalf("AddProfile: %v", err)
			}
			if _, err := k.SetProfileEnabled("ghost", false); err != nil {
				t.Fatalf("SetProfileEnabled: %v", err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prov := mock.New(
				mock.ToolUse("c1", "delegate", map[string]any{"task": "t", "agent": "ghost"}),
				mock.FinalText("lead adapted"), // the lead's next turn after the tool error
			)
			k := openSubAgentKernel(t, prov, 1)
			if tc.setup != nil {
				tc.setup(t, k)
			}
			col := &collector{}
			col.watch(k)
			ans, _, err := k.Run(context.Background(), "lead task")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if ans != "lead adapted" {
				t.Errorf("answer = %q", ans)
			}
			time.Sleep(50 * time.Millisecond)
			if n := len(col.ofKind(event.KindSubAgentSpawned)); n != 0 {
				t.Errorf("%d sub-agent(s) spawned for a refused agent ref", n)
			}
		})
	}
}
