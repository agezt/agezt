// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/skill"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/file"
	"github.com/agezt/agezt/plugins/tools/shell"
)

type toggledNotifyTool struct {
	fail atomic.Bool
}

func (t *toggledNotifyTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "notify", Description: "test notify", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t *toggledNotifyTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	if t.fail.Load() {
		return agent.Result{Output: "notify failed", IsError: true}, nil
	}
	return agent.Result{Output: "sent"}, nil
}

// TestWithAgentProfile_AppliesIdentityToRun: the one-call profile application
// (M790 — used by the standing runner) carries the whole identity into the
// run: soul → system, model + fallbacks → model chain, memory scope → private
// notes in the injected context.
func TestWithAgentProfile_AppliesIdentityToRun(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k, err := runtime.Open(runtime.Config{
		BaseDir:      t.TempDir(),
		Provider:     prov,
		Model:        "default-model",
		Tools:        map[string]agent.Tool{"shell": shell.New()},
		MemoryInject: true,
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

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "researcher", Soul: "You are Researcher.",
		Model: "agent-model", Fallbacks: []string{"agent-model", "backup-1"},
		MaxDailyMc: 5_000_000_000,
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "what about the target?"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}

	if req.Model != "agent-model" {
		t.Errorf("model = %q, want agent-model", req.Model)
	}
	if len(req.ModelChain) != 2 || req.ModelChain[0] != "agent-model" || req.ModelChain[1] != "backup-1" {
		t.Errorf("chain = %v, want [agent-model backup-1] (dupe skipped)", req.ModelChain)
	}
	if !strings.Contains(req.System, "You are Researcher.") {
		t.Errorf("soul missing from system:\n%s", req.System)
	}
	if !strings.Contains(req.System, "researcher-private-fact") {
		t.Errorf("memory scope not applied — private note missing:\n%s", req.System)
	}
	if req.Agent != "researcher" || req.AgentDailyCeilingMc != 5_000_000_000 {
		t.Errorf("identity ledger fields = %q/%d, want researcher/5e9 (M793)", req.Agent, req.AgentDailyCeilingMc)
	}
}

// TestWithAgentProfile_WorkdirConfinesFileTool: a profile workdir (M792) makes
// the run's file-tool writes land inside <workspace>/<workdir>.
func TestWithAgentProfile_WorkdirConfinesFileTool(t *testing.T) {
	ws := t.TempDir()
	ft, err := file.New(ws)
	if err != nil {
		t.Fatalf("file.New: %v", err)
	}
	prov := mock.New(
		mock.ToolUse("c1", "file", map[string]any{"op": "write", "path": "notes.txt", "content": "from researcher"}),
		mock.FinalText("done"),
	)
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Tools:    map[string]agent.Tool{"file": ft},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "researcher", Workdir: "research"})
	if got := agent.WorkdirFromContext(ctx); got != "research" {
		t.Fatalf("context workdir = %q, want research", got)
	}
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "take a note"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(ws, "research", "notes.txt"))
	if err != nil || string(b) != "from researcher" {
		t.Fatalf("write did not land in the agent's workdir: %v %q", err, b)
	}
}

func TestWithAgentProfile_WorkdirRejectsEscape(t *testing.T) {
	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{Slug: "escape", Workdir: "../outside"})
	if got := agent.WorkdirFromContext(ctx); got != "" {
		t.Fatalf("unsafe workdir propagated into tool context: %q", got)
	}
}

func TestWithAgentProfile_SystemAgentSkipsAutomaticLearningLayers(t *testing.T) {
	prov := mock.New(
		mock.ToolUse("c1", "shell", map[string]string{"command": "echo hi"}),
		mock.FinalText("done"),
	)
	var requests []agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { requests = append(requests, r) }
	k, err := runtime.Open(runtime.Config{
		BaseDir:               t.TempDir(),
		Provider:              prov,
		Model:                 "default-model",
		Tools:                 map[string]agent.Tool{"shell": shell.New()},
		System:                "base",
		MemoryInject:          true,
		MemoryDistill:         true,
		MemoryDistillMinTools: 1,
		SkillInject:           true,
		SkillForge:            true,
		SkillForgeMinTools:    1,
		ShadowEval:            true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.Memory().Remember("", memory.RememberSpec{
		Subject: "guardian private", Content: "system-agent-private-memory",
		Tags: map[string]string{"scope": "system/guardian-health"},
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}
	sk, _, err := k.Forge().Create("seed", skill.CreateSpec{
		Name: "guardian-procedure", Description: "guardian health", Triggers: []string{"health"},
		Body: "system-agent-private-skill", Agent: "guardian-health",
	})
	if err != nil {
		t.Fatal(err)
	}
	promoteToActive(t, k.Forge(), sk.ID)

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "guardian-health", Soul: "You are Guardian Health.", System: true,
		MemoryScope: "system/guardian-health",
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "check health"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if prov.CallCount() != 2 {
		t.Fatalf("system agent should not spend extra LLM calls on distill/forge, got %d calls", prov.CallCount())
	}
	for _, req := range requests {
		if strings.Contains(req.System, "system-agent-private-memory") {
			t.Fatalf("system agent auto-injected memory into prompt:\n%s", req.System)
		}
		if strings.Contains(req.System, "system-agent-private-skill") {
			t.Fatalf("system agent auto-injected skill into prompt:\n%s", req.System)
		}
	}
	allMemory, _ := k.Memory().All()
	if len(allMemory) != 1 {
		t.Fatalf("system agent should not auto-distill new memory, got %+v", allMemory)
	}
	allSkills, _ := k.Forge().List()
	if len(allSkills) != 1 {
		t.Fatalf("system agent should not auto-forge draft skills, got %+v", allSkills)
	}
}

func TestWithAgentProfile_IncludesInstructionsAndTasks(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		System:   "base",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug:         "ops",
		Soul:         "You are Ops.",
		Instructions: []string{"Stay quiet on green checks."},
		TaskList: []roster.AgentTask{
			{Title: "scan mailbox", Scope: "cycle", Status: "todo"},
			{Title: "finish cleanup", Scope: "total", Status: "doing"},
			{Title: "old task", Scope: "total", Status: "done"},
		},
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "wake"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	for _, want := range []string{"You are Ops.", "Standing instructions", "Stay quiet", "Cycle tasks", "scan mailbox", "Total tasks", "finish cleanup"} {
		if !strings.Contains(req.System, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, req.System)
		}
	}
	if strings.Contains(req.System, "old task") {
		t.Fatalf("completed tasks should not be injected:\n%s", req.System)
	}
}

func TestWithAgentProfile_RetireOnCompleteMovesAgentToGraveyard(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("done")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	p, err := k.AddProfile(roster.Profile{
		Slug:      "one-shot",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleRetireOnComplete},
	})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, err := k.RunWith(runtime.WithAgentProfile(context.Background(), p), k.NewCorrelation(), "finish once"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	got, ok := k.Roster().Get("one-shot")
	if !ok || !got.Retired || got.Enabled {
		t.Fatalf("one-shot agent should be retired and paused after completion: %+v ok=%v", got, ok)
	}
	if !strings.Contains(got.RetiredReason, "completed run") {
		t.Fatalf("retirement reason should mention completion, got %q", got.RetiredReason)
	}
}

func TestWithAgentProfile_CycleLifecycleIncrementsAndRetiresAtMax(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("done"), mock.FinalText("done")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	p, err := k.AddProfile(roster.Profile{
		Slug:      "cycler",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle, MaxCycles: 2},
		TaskList: []roster.AgentTask{
			{Title: "scan mailbox", Scope: "cycle", Status: "done"},
			{Title: "triage blockers", Scope: "cycle", Status: "blocked"},
			{Title: "finish migration", Scope: "total", Status: "done"},
		},
	})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	ctx := runtime.WithAgentProfile(context.Background(), p)
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "cycle once"); err != nil {
		t.Fatalf("RunWith first cycle: %v", err)
	}
	got, ok := k.Roster().Get("cycler")
	if !ok {
		t.Fatalf("cycler missing after first cycle")
	}
	if got.Lifecycle.CompletedCycles != 1 {
		t.Fatalf("completed cycles after first run = %d, want 1", got.Lifecycle.CompletedCycles)
	}
	if got.Retired || !got.Enabled {
		t.Fatalf("cycler should stay active before max cycles: %+v", got)
	}
	if status := taskStatus(got.TaskList, "scan mailbox"); status != "todo" {
		t.Fatalf("completed cycle task should reset for next wake, got %q", status)
	}
	if status := taskStatus(got.TaskList, "triage blockers"); status != "blocked" {
		t.Fatalf("blocked cycle task should stay blocked, got %q", status)
	}
	if status := taskStatus(got.TaskList, "finish migration"); status != "done" {
		t.Fatalf("total task should stay done, got %q", status)
	}
	var sawCycleEvent bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindRosterUpdated || e.CorrelationID == "" {
			return nil
		}
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		if pl["slug"] == "cycler" && pl["action"] == "lifecycle_cycle_completed" && int(pl["completed_cycles"].(float64)) == 1 {
			sawCycleEvent = true
		}
		return nil
	})
	if !sawCycleEvent {
		t.Fatal("cycle completion event was not journaled")
	}

	if _, err := k.RunWith(ctx, k.NewCorrelation(), "cycle twice"); err != nil {
		t.Fatalf("RunWith second cycle: %v", err)
	}
	got, ok = k.Roster().Get("cycler")
	if !ok {
		t.Fatalf("cycler missing after second cycle")
	}
	if got.Lifecycle.CompletedCycles != 2 {
		t.Fatalf("completed cycles after second run = %d, want 2", got.Lifecycle.CompletedCycles)
	}
	if !got.Retired || got.Enabled {
		t.Fatalf("cycler should retire after max cycles: %+v", got)
	}
	if !strings.Contains(got.RetiredReason, "completed 2/2 cycles") {
		t.Fatalf("retirement reason should mention cycle limit, got %q", got.RetiredReason)
	}
}

func TestCompleteAgentLifecycle_IdempotentPerCorrelation(t *testing.T) {
	// Guards against the RunAssured/RunWithRetry over-increment: those re-run the
	// agent under ONE correlation until the work verifies complete, and each inner
	// success calls completeAgentLifecycle. The cycle must advance once per logical
	// run (correlation), not once per inner attempt.
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("done")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	p, err := k.AddProfile(roster.Profile{
		Slug:      "cycler",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle, MaxCycles: 3},
	})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	ctx := runtime.WithAgentProfile(context.Background(), p)

	corr1 := k.NewCorrelation()
	k.CompleteAgentLifecycle(ctx, corr1)
	k.CompleteAgentLifecycle(ctx, corr1) // same logical run: must NOT advance again
	k.CompleteAgentLifecycle(ctx, corr1)
	got, ok := k.Roster().Get("cycler")
	if !ok {
		t.Fatal("cycler missing")
	}
	if got.Lifecycle.CompletedCycles != 1 {
		t.Fatalf("repeated completion under one correlation advanced cycle to %d, want 1", got.Lifecycle.CompletedCycles)
	}
	if got.Lifecycle.LastCompletedRun != corr1 {
		t.Fatalf("last completed run = %q, want %q", got.Lifecycle.LastCompletedRun, corr1)
	}

	// A genuinely new logical run (new correlation) advances again.
	corr2 := k.NewCorrelation()
	k.CompleteAgentLifecycle(ctx, corr2)
	got, _ = k.Roster().Get("cycler")
	if got.Lifecycle.CompletedCycles != 2 {
		t.Fatalf("new correlation advanced cycle to %d, want 2", got.Lifecycle.CompletedCycles)
	}

	// Reaching max retires the agent — and a duplicate of the maxing correlation
	// does not resurrect or re-advance it.
	corr3 := k.NewCorrelation()
	k.CompleteAgentLifecycle(ctx, corr3)
	k.CompleteAgentLifecycle(ctx, corr3)
	got, _ = k.Roster().Get("cycler")
	if got.Lifecycle.CompletedCycles != 3 {
		t.Fatalf("third correlation advanced cycle to %d, want 3", got.Lifecycle.CompletedCycles)
	}
	if !got.Retired {
		t.Fatalf("cycler should retire at max cycles: %+v", got.Lifecycle)
	}
}

func taskStatus(tasks []roster.AgentTask, title string) string {
	for _, task := range tasks {
		if task.Title == title {
			return task.Status
		}
	}
	return ""
}

func TestWithAgentProfile_LifecycleDoesNotCompleteOnFailedRun(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	oneShot, err := k.AddProfile(roster.Profile{
		Slug:      "one-shot-failed",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleRetireOnComplete},
	})
	if err != nil {
		t.Fatalf("AddProfile one-shot: %v", err)
	}
	if _, err := k.RunWith(runtime.WithAgentProfile(context.Background(), oneShot), k.NewCorrelation(), "finish once"); err == nil {
		t.Fatal("RunWith one-shot should fail with exhausted mock provider")
	}
	got, ok := k.Roster().Get("one-shot-failed")
	if !ok {
		t.Fatal("one-shot-failed missing after failed run")
	}
	if got.Retired || !got.Enabled {
		t.Fatalf("failed one-shot run must not retire or pause the agent: %+v", got)
	}

	cycler, err := k.AddProfile(roster.Profile{
		Slug:      "cycler-failed",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle, MaxCycles: 2},
	})
	if err != nil {
		t.Fatalf("AddProfile cycler: %v", err)
	}
	if _, err := k.RunWith(runtime.WithAgentProfile(context.Background(), cycler), k.NewCorrelation(), "cycle once"); err == nil {
		t.Fatal("RunWith cycler should fail with exhausted mock provider")
	}
	got, ok = k.Roster().Get("cycler-failed")
	if !ok {
		t.Fatal("cycler-failed missing after failed run")
	}
	if got.Lifecycle.CompletedCycles != 0 {
		t.Fatalf("failed cycle run completed cycles = %d, want 0", got.Lifecycle.CompletedCycles)
	}
	if got.Retired || !got.Enabled {
		t.Fatalf("failed cycle run must not retire or pause the agent: %+v", got)
	}
}

func TestWithAgentProfile_ToolPermissionsApply(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k, err := runtime.Open(runtime.Config{
		BaseDir:    t.TempDir(),
		Provider:   prov,
		Tools:      map[string]agent.Tool{"shell": shell.New()},
		MemoryTool: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug:      "guarded",
		ToolAllow: []string{"shell", "memory"},
		ToolDeny:  []string{"shell"},
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "hello"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "memory" {
		names := make([]string, 0, len(req.Tools))
		for _, tdef := range req.Tools {
			names = append(names, tdef.Name)
		}
		t.Fatalf("tool policy should leave only memory, got %v", names)
	}
}

func TestWithAgentProfile_NoisePolicyDisablesMemoryTool(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k, err := runtime.Open(runtime.Config{
		BaseDir:    t.TempDir(),
		Provider:   prov,
		MemoryTool: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "quiet",
		NoisePolicy: &roster.NoisePolicy{
			DisableMemoryWrites: true,
		},
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "hello"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	for _, tdef := range req.Tools {
		if tdef.Name == "memory" {
			t.Fatalf("memory tool should be removed by noise policy, got tools %+v", req.Tools)
		}
	}
}

func TestWithAgentProfile_NoisePolicyBlocksDirectMemoryWrites(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:    t.TempDir(),
		Provider:   mock.New(mock.FinalText("ok")),
		MemoryTool: true,
		Edict:      edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "quiet",
		NoisePolicy: &roster.NoisePolicy{
			DisableMemoryWrites: true,
		},
	})
	if _, err := k.RunTool(ctx, "corr", "write", "memory", json.RawMessage(`{"action":"remember","subject":"s","content":"should not persist"}`)); err == nil || !strings.Contains(err.Error(), "memory writes are disabled") {
		t.Fatalf("direct memory write err = %v, want noise-policy denial", err)
	}
	if _, err := k.RunTool(ctx, "corr", "read", "memory", json.RawMessage(`{"action":"recall","query":"s"}`)); err != nil {
		t.Fatalf("direct memory recall should remain allowed: %v", err)
	}
}

func TestWithAgentProfile_ToolPermissionsGateDirectToolRuns(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"alpha": quietTool{name: "alpha"},
			"beta":  quietTool{name: "beta"},
		},
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug:      "guarded",
		ToolAllow: []string{"alpha"},
		ToolDeny:  []string{"alpha"},
	})
	if _, err := k.RunTool(ctx, "corr", "call-alpha", "alpha", json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("denylist direct tool err = %v, want agent tool denylist", err)
	}
	if _, err := k.RunTool(ctx, "corr", "call-beta", "beta", json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "not in agent tool allowlist") {
		t.Fatalf("allowlist direct tool err = %v, want not in agent tool allowlist", err)
	}
}

func TestWithAgentProfile_NoisePolicyGatesNotifySeverityAndCooldown(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"notify": quietTool{name: "notify"},
		},
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "guardian-health",
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity:    "warning",
			MinNotifyIntervalSec: 3600,
		},
	})
	if _, err := k.RunTool(ctx, "corr", "info", "notify", json.RawMessage(`{"text":"green"}`)); err == nil || !strings.Contains(err.Error(), "severity must be at least warning") {
		t.Fatalf("info notify err = %v, want severity gate", err)
	}
	if _, err := k.RunTool(ctx, "corr", "warn", "notify", json.RawMessage(`{"text":"changed routing","severity":"warning"}`)); err != nil {
		t.Fatalf("warning notify should pass: %v", err)
	}
	if _, err := k.RunTool(ctx, "corr", "warn2", "notify", json.RawMessage(`{"text":"again","severity":"critical"}`)); err == nil || !strings.Contains(err.Error(), "notify cooldown active") {
		t.Fatalf("second notify err = %v, want cooldown gate", err)
	}
}

func TestWithAgentProfile_NoisePolicyDoesNotCooldownFailedNotify(t *testing.T) {
	notify := &toggledNotifyTool{}
	notify.fail.Store(true)
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"notify": notify,
		},
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "guardian-health",
		NoisePolicy: &roster.NoisePolicy{
			MinNotifySeverity:    "warning",
			MinNotifyIntervalSec: 3600,
		},
	})
	if res, err := k.RunTool(ctx, "corr", "warn", "notify", json.RawMessage(`{"text":"changed routing","severity":"warning"}`)); err != nil || !res.IsError {
		t.Fatalf("failed notify result = %+v err=%v, want IsError result", res, err)
	}
	notify.fail.Store(false)
	if _, err := k.RunTool(ctx, "corr", "warn2", "notify", json.RawMessage(`{"text":"still needs attention","severity":"warning"}`)); err != nil {
		t.Fatalf("successful notify after failed send should not be cooldown-blocked: %v", err)
	}
	if _, err := k.RunTool(ctx, "corr", "warn3", "notify", json.RawMessage(`{"text":"again","severity":"critical"}`)); err == nil || !strings.Contains(err.Error(), "notify cooldown active") {
		t.Fatalf("notify after successful send err = %v, want cooldown gate", err)
	}
}

func TestWithAgentProfile_SilentOnSuccessBlocksRoutineNotify(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools: map[string]agent.Tool{
			"notify": quietTool{name: "notify"},
		},
		Edict: edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "quiet-worker",
		NoisePolicy: &roster.NoisePolicy{
			SilentOnSuccess: true,
		},
	})
	if _, err := k.RunTool(ctx, "corr", "info", "notify", json.RawMessage(`{"text":"green check passed"}`)); err == nil || !strings.Contains(err.Error(), "severity must be at least warning") {
		t.Fatalf("info notify err = %v, want silent-success severity gate", err)
	}
	if _, err := k.RunTool(ctx, "corr", "warn", "notify", json.RawMessage(`{"text":"needs attention","severity":"warning"}`)); err != nil {
		t.Fatalf("warning notify should pass under silent_on_success: %v", err)
	}
}

func TestWithAgentProfile_ConfigOverrideChangesModel(t *testing.T) {
	prov := mock.New(mock.FinalText("ok"))
	var req agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { req = r }
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
		Model:    "default-model",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "config-agent",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL": "override-model",
		},
	})
	if _, err := k.RunWith(ctx, k.NewCorrelation(), "hello"); err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if req.Model != "override-model" {
		t.Fatalf("model = %q, want override-model", req.Model)
	}
}

func TestWithAgentProfile_ConfigOverrideCanDisableHeuristicBypass(t *testing.T) {
	prov := mock.New(mock.FinalText("provider-answer"))
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ctx := runtime.WithAgentProfile(context.Background(), roster.Profile{
		Slug: "config-agent",
		ConfigOverrides: map[string]string{
			"AGEZT_DISABLE_HEURISTIC_BYPASS": "on",
		},
	})
	answer, err := k.RunWith(ctx, k.NewCorrelation(), "what time is it")
	if err != nil {
		t.Fatalf("RunWith: %v", err)
	}
	if answer != "provider-answer" {
		t.Fatalf("answer = %q, want provider-answer", answer)
	}
	if prov.CallCount() != 1 {
		t.Fatalf("provider should have been called once, got %d", prov.CallCount())
	}
}
