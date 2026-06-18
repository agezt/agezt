// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
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
		Slug:         "researcher",
		Soul:         "You are Researcher. Cite sources.",
		Model:        "agent-model",
		Instructions: []string{"Check the mailbox before reporting."},
		TaskList: []roster.AgentTask{
			{Title: "triage sources", Scope: "cycle", Status: "todo"},
			{Title: "complete dossier", Scope: "total", Status: "blocked"},
			{Title: "old finished work", Scope: "total", Status: "done"},
		},
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
	for _, want := range []string{
		"Standing instructions",
		"Check the mailbox before reporting.",
		"Cycle tasks",
		"triage sources",
		"Total tasks",
		"complete dossier",
	} {
		if !strings.Contains(child.System, want) {
			t.Errorf("child system prompt missing %q: %q", want, child.System)
		}
	}
	if strings.Contains(child.System, "old finished work") {
		t.Errorf("child system prompt included completed task: %q", child.System)
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

func TestDelegate_NamedAgentToolPolicyFiltersChildTools(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "restricted work", "agent": "worker"}),
		mock.FinalText("child done"),
		mock.FinalText("lead done"),
	)
	var reqs []agent.CompletionRequest
	prov.OnRequest = func(req agent.CompletionRequest) { reqs = append(reqs, req) }
	k := openSubAgentKernel(t, prov, 1)

	if _, err := k.AddProfile(roster.Profile{
		Slug:     "worker",
		Soul:     "Restricted worker.",
		Model:    "worker-model",
		ToolDeny: []string{"shell"},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var child *agent.CompletionRequest
	for i := range reqs {
		if reqs[i].Model == "worker-model" {
			child = &reqs[i]
			break
		}
	}
	if child == nil {
		t.Fatalf("child request with worker model not seen; requests=%d", len(reqs))
	}
	names := toolNames(child.Tools)
	if contains(names, "shell") {
		t.Fatalf("child tools = %v, want shell filtered by named agent denylist", names)
	}
	if !contains(names, "delegate") {
		t.Fatalf("child tools = %v, want allowed delegate retained", names)
	}
}

func TestDelegate_NamedAgentNoiseNotifyCompletesCooldown(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "notify once", "agent": "worker"}),
		mock.ToolUse("c2", "notify", map[string]any{"text": "needs attention", "severity": "warning"}),
		mock.FinalText("child done"),
		mock.FinalText("lead done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:          t.TempDir(),
		Provider:         prov,
		Tools:            map[string]agent.Tool{"shell": shell.New(), "notify": quietTool{name: "notify"}},
		SubAgentTool:     true,
		SubAgentMaxDepth: 1,
		Edict:            edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddProfile(roster.Profile{
		Slug:  "worker",
		Model: "worker-model",
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity:    "warning",
			MinNotifyIntervalSec: 3600,
		},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "worker",
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity:    "warning",
			MinNotifyIntervalSec: 3600,
		},
	})
	if _, err := k.RunTool(ctx, "corr", "warn2", "notify", json.RawMessage(`{"text":"again","severity":"critical"}`)); err == nil || !strings.Contains(err.Error(), "notify cooldown active") {
		t.Fatalf("notify after sub-agent send err = %v, want cooldown active", err)
	}
}

func TestDelegate_NamedAgentCycleLifecycleCompletes(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "complete cycle", "agent": "worker"}),
		mock.FinalText("child done"),
		mock.FinalText("lead done"),
	)
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{
		Slug:      "worker",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle, MaxCycles: 1},
		TaskList: []roster.AgentTask{
			{Title: "cycle task", Scope: "cycle", Status: "done"},
			{Title: "blocked task", Scope: "cycle", Status: "blocked"},
		},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, ok := k.Roster().Get("worker")
	if !ok {
		t.Fatal("worker missing after delegated run")
	}
	if got.Lifecycle.CompletedCycles != 1 {
		t.Fatalf("completed cycles = %d, want 1", got.Lifecycle.CompletedCycles)
	}
	if !got.Retired || got.Enabled {
		t.Fatalf("worker should retire after delegated max cycle, got enabled=%v retired=%v", got.Enabled, got.Retired)
	}
	if !strings.Contains(got.RetiredReason, "completed 1/1 cycles") {
		t.Fatalf("retirement reason = %q, want completed cycle limit", got.RetiredReason)
	}
	if status := taskStatus(got.TaskList, "cycle task"); status != "todo" {
		t.Fatalf("completed cycle task status = %q, want todo", status)
	}
	if status := taskStatus(got.TaskList, "blocked task"); status != "blocked" {
		t.Fatalf("blocked cycle task status = %q, want blocked", status)
	}
}

func TestDelegate_ManagedSubAgentRequiresParent(t *testing.T) {
	directFalse := false
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "work", "agent": "worker"}),
		mock.FinalText("child done"),
		mock.FinalText("lead done"),
	)
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{
		Slug: "lead", Soul: "Lead the workers.",
	}); err != nil {
		t.Fatalf("AddProfile lead: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{
		Slug: "worker", Soul: "Do worker tasks.", ParentAgent: "lead", DirectCallable: &directFalse,
	}); err != nil {
		t.Fatalf("AddProfile worker: %v", err)
	}

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "lead", Soul: "Lead the workers."})
	ans, _, err := k.Run(ctx, "lead task")
	if err != nil {
		t.Fatalf("Run as lead: %v", err)
	}
	if ans != "lead done" {
		t.Fatalf("answer = %q, want lead done", ans)
	}

	prov2 := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "work", "agent": "worker"}),
		mock.FinalText("other done"),
	)
	k2 := openSubAgentKernel(t, prov2, 1)
	if _, err := k2.AddProfile(roster.Profile{Slug: "lead"}); err != nil {
		t.Fatalf("AddProfile lead k2: %v", err)
	}
	if _, err := k2.AddProfile(roster.Profile{
		Slug: "worker", ParentAgent: "lead", DirectCallable: &directFalse,
	}); err != nil {
		t.Fatalf("AddProfile worker k2: %v", err)
	}
	col := &collector{}
	col.watch(k2)
	ctx = runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "other"})
	if _, _, err := k2.Run(ctx, "other task"); err != nil {
		t.Fatalf("Run as other: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := len(col.ofKind(event.KindSubAgentSpawned)); got != 0 {
		t.Fatalf("unauthorized delegation spawned %d sub-agent(s), want 0", got)
	}
}

func TestDelegate_ManagedSubAgentAllowsOwner(t *testing.T) {
	directFalse := false
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "work", "agent": "worker"}),
		mock.FinalText("child done"),
		mock.FinalText("owner done"),
	)
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{
		Slug: "owner", Soul: "Own the workers.",
	}); err != nil {
		t.Fatalf("AddProfile owner: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{
		Slug: "worker", Soul: "Do worker tasks.", OwnerAgent: "owner", DirectCallable: &directFalse,
	}); err != nil {
		t.Fatalf("AddProfile worker: %v", err)
	}

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "owner", Soul: "Own the workers."})
	ans, _, err := k.Run(ctx, "owner task")
	if err != nil {
		t.Fatalf("Run as owner: %v", err)
	}
	if ans != "owner done" {
		t.Fatalf("answer = %q, want owner done", ans)
	}

	prov2 := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "work", "agent": "worker"}),
		mock.FinalText("other done"),
	)
	k2 := openSubAgentKernel(t, prov2, 1)
	if _, err := k2.AddProfile(roster.Profile{Slug: "owner"}); err != nil {
		t.Fatalf("AddProfile owner k2: %v", err)
	}
	if _, err := k2.AddProfile(roster.Profile{
		Slug: "worker", OwnerAgent: "owner", DirectCallable: &directFalse,
	}); err != nil {
		t.Fatalf("AddProfile worker k2: %v", err)
	}
	col := &collector{}
	col.watch(k2)
	ctx = runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "other"})
	if _, _, err := k2.Run(ctx, "other task"); err != nil {
		t.Fatalf("Run as other: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if got := len(col.ofKind(event.KindSubAgentSpawned)); got != 0 {
		t.Fatalf("unauthorized owner-managed delegation spawned %d sub-agent(s), want 0", got)
	}
}

func TestDelegate_ManagedSubAgentRequiresLiveParent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtime.Kernel, *testing.T)
	}{
		{
			name: "paused parent",
			mutate: func(k *runtime.Kernel, t *testing.T) {
				t.Helper()
				if _, err := k.SetProfileEnabled("lead", false); err != nil {
					t.Fatalf("pause lead: %v", err)
				}
			},
		},
		{
			name: "retired parent",
			mutate: func(k *runtime.Kernel, t *testing.T) {
				t.Helper()
				if _, err := k.SetProfileRetired("lead", true, "manager retired"); err != nil {
					t.Fatalf("retire lead: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directFalse := false
			prov := mock.New(
				mock.ToolUse("c1", "delegate", map[string]any{"task": "work", "agent": "worker"}),
				mock.FinalText("lead done"),
			)
			k := openSubAgentKernel(t, prov, 1)
			if _, err := k.AddProfile(roster.Profile{Slug: "lead", Soul: "Lead the workers."}); err != nil {
				t.Fatalf("AddProfile lead: %v", err)
			}
			if _, err := k.AddProfile(roster.Profile{
				Slug: "worker", Soul: "Do worker tasks.", ParentAgent: "lead", DirectCallable: &directFalse,
			}); err != nil {
				t.Fatalf("AddProfile worker: %v", err)
			}
			tc.mutate(k, t)

			col := &collector{}
			col.watch(k)
			ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "lead", Soul: "Lead the workers."})
			if _, _, err := k.Run(ctx, "lead task"); err != nil {
				t.Fatalf("Run as lead: %v", err)
			}
			time.Sleep(50 * time.Millisecond)
			if got := len(col.ofKind(event.KindSubAgentSpawned)); got != 0 {
				t.Fatalf("delegation through inactive manager spawned %d sub-agent(s), want 0", got)
			}
		})
	}
}

func TestDelegate_ManagedSubAgentRequiresLiveOwner(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtime.Kernel, *testing.T)
	}{
		{
			name: "paused owner",
			mutate: func(k *runtime.Kernel, t *testing.T) {
				t.Helper()
				if _, err := k.SetProfileEnabled("owner", false); err != nil {
					t.Fatalf("pause owner: %v", err)
				}
			},
		},
		{
			name: "retired owner",
			mutate: func(k *runtime.Kernel, t *testing.T) {
				t.Helper()
				if _, err := k.SetProfileRetired("owner", true, "manager retired"); err != nil {
					t.Fatalf("retire owner: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directFalse := false
			prov := mock.New(
				mock.ToolUse("c1", "delegate", map[string]any{"task": "work", "agent": "worker"}),
				mock.FinalText("owner done"),
			)
			k := openSubAgentKernel(t, prov, 1)
			if _, err := k.AddProfile(roster.Profile{Slug: "owner", Soul: "Own the workers."}); err != nil {
				t.Fatalf("AddProfile owner: %v", err)
			}
			if _, err := k.AddProfile(roster.Profile{
				Slug: "worker", Soul: "Do worker tasks.", OwnerAgent: "owner", DirectCallable: &directFalse,
			}); err != nil {
				t.Fatalf("AddProfile worker: %v", err)
			}
			tc.mutate(k, t)

			col := &collector{}
			col.watch(k)
			ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "owner", Soul: "Own the workers."})
			if _, _, err := k.Run(ctx, "owner task"); err != nil {
				t.Fatalf("Run as owner: %v", err)
			}
			time.Sleep(50 * time.Millisecond)
			if got := len(col.ofKind(event.KindSubAgentSpawned)); got != 0 {
				t.Fatalf("delegation through inactive owner spawned %d sub-agent(s), want 0", got)
			}
		})
	}
}

// TestDelegate_AgentMemoryScopeFollowsChild: the sub-agent's memory-tool
// recalls default to the profile's scope (M786) — its private notes surface
// without the child naming itself.
func TestDelegate_AgentMemoryScopeFollowsChild(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "check notes", "agent": "researcher"}),
		mock.ToolUse("c2", "memory", map[string]any{"action": "recall", "query": "target"}), // child turn 1
		mock.FinalText("child done"), // child turn 2
		mock.FinalText("lead done"),  // lead final
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:          t.TempDir(),
		Provider:         prov,
		Tools:            map[string]agent.Tool{"shell": shell.New()},
		SubAgentTool:     true,
		SubAgentMaxDepth: 1,
		MemoryTool:       true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Memory().Remember("", memory.RememberSpec{
		Subject: "target notes", Content: "researcher-private-fact",
		Tags: map[string]string{"scope": "researcher"},
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}
	if _, err := k.AddProfile(roster.Profile{Slug: "researcher"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	col := &collector{}
	col.watch(k)
	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// The child's memory recall surfaced the scope-private note.
	found := false
	for _, e := range col.ofKind(event.KindToolResult) {
		if strings.Contains(string(e.Payload), "researcher-private-fact") {
			found = true
			break
		}
	}
	if !found {
		t.Error("child's memory recall did not surface the agent's private note")
	}
}

func TestDelegate_NamedAgentInjectsMemoryAndSkills(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "handle project atlas ci", "agent": "researcher"}),
		mock.FinalText("child done"),
		mock.FinalText("lead done"),
	)
	var childSystem string
	prov.OnRequest = func(req agent.CompletionRequest) {
		if strings.Contains(req.System, "focused sub-agent") {
			childSystem = req.System
		}
	}
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     prov,
		Tools:        map[string]agent.Tool{"shell": shell.New()},
		SubAgentTool: true,
		MemoryInject: true,
		SkillInject:  true,
		MemoryTopK:   5,
		SkillTopK:    3,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddProfile(roster.Profile{Slug: "researcher"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Type:    memory.TypeFact,
		Subject: "project atlas ci",
		Content: "project atlas uses the neon CI lane",
		Tags:    map[string]string{"scope": "researcher"},
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}
	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name:        "atlas-ci",
		Description: "project atlas ci",
		Triggers:    []string{"atlas", "ci"},
		Body:        "Always check the neon CI lane first.",
		Agent:       "researcher",
	})
	if err != nil {
		t.Fatalf("Create skill: %v", err)
	}
	promoteToActive(t, k.Forge(), sk.ID)

	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(childSystem, "project atlas uses the neon CI lane") {
		t.Fatalf("child system missing injected private memory:\n%s", childSystem)
	}
	if !strings.Contains(childSystem, "Always check the neon CI lane first.") {
		t.Fatalf("child system missing injected private skill:\n%s", childSystem)
	}
	if countKind(t, k, event.KindMemoryRetrieved) == 0 {
		t.Fatal("sub-agent memory injection must journal memory.retrieved")
	}
	if countKind(t, k, event.KindSkillActivated) == 0 {
		t.Fatal("sub-agent skill injection must journal skill.activated")
	}
}

// TestDelegate_AgentModelChainFollowsChild: the child's completion requests
// carry the profile's fallback chain (M787), primary first, dupes skipped.
func TestDelegate_AgentModelChainFollowsChild(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "t", "agent": "researcher"}),
		mock.FinalText("child"),
		mock.FinalText("lead"),
	)
	var chains [][]string
	prov.OnRequest = func(req agent.CompletionRequest) {
		if req.Model == "agent-model" { // the child's request
			chains = append(chains, req.ModelChain)
		}
	}
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{
		Slug: "researcher", Model: "agent-model",
		Fallbacks: []string{"agent-model", "backup-1"}, // dupe of primary must be skipped
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("child request never seen")
	}
	want := []string{"agent-model", "backup-1"}
	if len(chains[0]) != 2 || chains[0][0] != want[0] || chains[0][1] != want[1] {
		t.Errorf("child chain = %v, want %v", chains[0], want)
	}
}

func TestDelegate_AgentConfigOverrideModelFollowsChild(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{"task": "t", "agent": "researcher"}),
		mock.FinalText("child"),
		mock.FinalText("lead"),
	)
	var models []string
	var chains [][]string
	prov.OnRequest = func(req agent.CompletionRequest) {
		models = append(models, req.Model)
		if req.Model == "override-model" {
			chains = append(chains, req.ModelChain)
		}
	}
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{
		Slug: "researcher",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL": "override-model",
		},
		Fallbacks: []string{"backup-1"},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, m := range models {
		if m == "override-model" {
			if len(chains) == 0 || len(chains[0]) != 2 || chains[0][0] != "override-model" || chains[0][1] != "backup-1" {
				t.Fatalf("child override model chain = %v, want [override-model backup-1]", chains)
			}
			return
		}
	}
	t.Fatalf("config override model never reached the child provider; models: %v", models)
}

func TestDelegate_ExplicitModelWinsOverConfigOverride(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "delegate", map[string]any{
			"task": "t", "agent": "researcher", "model": "explicit-model"}),
		mock.FinalText("child"),
		mock.FinalText("lead"),
	)
	var models []string
	prov.OnRequest = func(req agent.CompletionRequest) { models = append(models, req.Model) }
	k := openSubAgentKernel(t, prov, 1)
	if _, err := k.AddProfile(roster.Profile{
		Slug: "researcher",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL": "override-model",
		},
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, _, err := k.Run(context.Background(), "lead task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, m := range models {
		if m == "override-model" {
			t.Fatalf("config override model used despite explicit delegate model; models: %v", models)
		}
	}
	for _, m := range models {
		if m == "explicit-model" {
			return
		}
	}
	t.Fatalf("explicit model never reached the child provider; models: %v", models)
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
