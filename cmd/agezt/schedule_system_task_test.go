// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type scheduledProbeTool struct {
	calls atomic.Int32
}

func (t *scheduledProbeTool) Definition() agent.ToolDef {
	return agent.ToolDef{Name: "probe", Description: "probe", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (t *scheduledProbeTool) Invoke(context.Context, json.RawMessage) (agent.Result, error) {
	t.calls.Add(1)
	return agent.Result{Output: "ok"}, nil
}

const scheduleCatalogSyncFixture = `{
  "testprov": {
    "id": "testprov",
    "name": "Test Provider",
    "env": ["TESTPROV_API_KEY"],
    "npm": "@ai-sdk/openai-compatible",
    "api": "https://api.testprov.invalid/v1",
    "models": {
      "test-model-1": {
        "id": "test-model-1",
        "name": "Test Model 1",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  }
}`

func TestScheduleFiredEventPayloadCarriesTypedTargetIdentity(t *testing.T) {
	workflowPayload := scheduleFiredEventPayload("sched-flow", "run nightly", "gpt-5", cadence.Entry{
		Target:   cadence.TargetWorkflow,
		Workflow: "nightly",
		Agent:    "ops",
	})
	if workflowPayload["schedule_id"] != "sched-flow" || workflowPayload["intent"] != "run nightly" || workflowPayload["model"] != "gpt-5" {
		t.Fatalf("workflow payload missing common fields: %v", workflowPayload)
	}
	if workflowPayload["target"] != cadence.TargetWorkflow || workflowPayload["workflow"] != "nightly" || workflowPayload["agent"] != "ops" {
		t.Fatalf("workflow payload missing typed identity: %v", workflowPayload)
	}
	if workflowPayload["executor"] != "workflow" || workflowPayload["uses_llm"] != true {
		t.Fatalf("workflow payload execution metadata wrong: %v", workflowPayload)
	}
	agentPayload := scheduleFiredEventPayload("sched-agent", "wake ops", "gpt-5", cadence.Entry{
		Target: cadence.TargetIntent,
		Agent:  "ops",
	}, &roster.Profile{
		Slug:    "ops",
		Enabled: true,
		Soul:    "Ops agent.",
		RetryPolicy: &roster.RetryPolicy{
			MaxAttempts: 3,
		},
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle},
	})
	runbook, _ := agentPayload["autonomy_runbook"].(map[string]any)
	if runbook["identity_kind"] != "custom" ||
		runbook["trigger_contract"] != "operator_schedule_channel" ||
		runbook["route_contract"] != "self_owned" ||
		runbook["recovery_contract"] != "retry" ||
		runbook["sleep_contract"] != roster.LifecycleCycle ||
		runbook["retry_attempts"] != 3 {
		t.Fatalf("agent schedule payload autonomy runbook wrong: %+v", runbook)
	}

	toolPayload := scheduleFiredEventPayload("sched-tool", "invoke tool", "gpt-5-mini", cadence.Entry{
		Target: cadence.TargetTool,
		Tool:   "memory",
		Agent:  "scout",
	})
	if toolPayload["target"] != cadence.TargetTool || toolPayload["tool"] != "memory" || toolPayload["agent"] != "scout" || toolPayload["model"] != "gpt-5-mini" {
		t.Fatalf("tool payload missing typed identity: %v", toolPayload)
	}
	if toolPayload["executor"] != "tool" || toolPayload["uses_llm"] != false {
		t.Fatalf("tool payload execution metadata wrong: %v", toolPayload)
	}

	systemTaskPayload := scheduleFiredEventPayload("sched-sync", "sync catalog", "", cadence.Entry{
		Target:     cadence.TargetSystemTask,
		SystemTask: cadence.SystemTaskCatalogSync,
	})
	if systemTaskPayload["target"] != cadence.TargetSystemTask || systemTaskPayload["system_task"] != cadence.SystemTaskCatalogSync {
		t.Fatalf("system task payload missing typed identity: %v", systemTaskPayload)
	}
	if systemTaskPayload["agent"] != "" {
		t.Fatalf("system task payload agent = %v, want empty", systemTaskPayload["agent"])
	}
	if systemTaskPayload["executor"] != "daemon" || systemTaskPayload["category"] != "catalog" || systemTaskPayload["effect_class"] != "config_update" || systemTaskPayload["uses_llm"] != false {
		t.Fatalf("system task payload execution metadata wrong: %v", systemTaskPayload)
	}
}

func TestScheduledRunContextModelOverrideWinsOverAgentProfile(t *testing.T) {
	prov := mock.New(mock.FinalText("done"))
	var llmReq agent.CompletionRequest
	prov.OnRequest = func(r agent.CompletionRequest) { llmReq = r }
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: prov,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.SaveWorkflow("", workflow.Workflow{
		Name: "scheduled-profile-flow",
		Nodes: []workflow.Node{
			{ID: "start", Type: workflow.NodeTrigger},
			{ID: "brief", Type: workflow.NodeLLM, Config: json.RawMessage(`{"prompt":"run"}`)},
		},
		Edges: []workflow.Edge{{From: "start", To: "brief"}},
	}); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	prof := roster.Profile{
		Slug:      "ops",
		Enabled:   true,
		Model:     "agent-model",
		Fallbacks: []string{"agent-model", "backup-model"},
	}
	ctx := scheduledRunContext(context.Background(), "schedule-model", &prof)
	if _, err := k.RunWorkflow(ctx, k.NewCorrelation(), "scheduled-profile-flow", nil); err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}
	if llmReq.Model != "schedule-model" {
		t.Fatalf("llm model = %q, want schedule-model", llmReq.Model)
	}
}

func TestRunScheduledTrackedTargetPublishesTaskLifecycle(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ent := cadence.Entry{ID: "sched-tool", Target: cadence.TargetTool, Tool: "memory"}
	if err := runScheduledTrackedTarget(context.Background(), k, "corr-tool", ent, "sync memory", func(context.Context) (string, error) {
		return "tool memory completed", nil
	}); err != nil {
		t.Fatalf("runScheduledTrackedTarget: %v", err)
	}

	var received, completed bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "corr-tool" || e.Actor != "schedule" {
			return nil
		}
		switch e.Kind {
		case event.KindTaskReceived:
			received = true
			var payload map[string]any
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatalf("received payload unmarshal: %v", err)
			}
			if payload["schedule_id"] != "sched-tool" || payload["target"] != cadence.TargetTool || payload["tool"] != "memory" {
				t.Fatalf("received payload missing schedule target fields: %v", payload)
			}
			if payload["intent"] != "run tool memory" {
				t.Fatalf("received intent = %v, want run tool memory", payload["intent"])
			}
		case event.KindTaskCompleted:
			completed = true
			var payload map[string]any
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatalf("completed payload unmarshal: %v", err)
			}
			if payload["answer"] != "tool memory completed" {
				t.Fatalf("completed answer = %v", payload["answer"])
			}
		}
		return nil
	})
	if !received || !completed {
		t.Fatalf("schedule typed target lifecycle missing received=%v completed=%v", received, completed)
	}
}

func TestRunScheduledTrackedTargetPublishesTaskFailure(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ent := cadence.Entry{ID: "sched-workflow", Target: cadence.TargetWorkflow, Workflow: "nightly"}
	if err := runScheduledTrackedTarget(context.Background(), k, "corr-workflow", ent, "", func(context.Context) (string, error) {
		return "", errors.New("boom")
	}); err == nil {
		t.Fatalf("runScheduledTrackedTarget succeeded, want error")
	}

	var failed bool
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "corr-workflow" || e.Kind != event.KindTaskFailed {
			return nil
		}
		failed = true
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("failed payload unmarshal: %v", err)
		}
		if payload["schedule_id"] != "sched-workflow" || payload["target"] != cadence.TargetWorkflow {
			t.Fatalf("failed payload missing schedule target fields: %v", payload)
		}
		if payload["reason"] != "error" || payload["error"] != "boom" {
			t.Fatalf("failed payload reason/error = %v/%v, want error/boom", payload["reason"], payload["error"])
		}
		return nil
	})
	if !failed {
		t.Fatalf("schedule typed target failure not journaled")
	}
}

func TestRunScheduledTrackedTargetCompletesAgentLifecycle(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	p, err := k.AddProfile(roster.Profile{
		Slug:      "cycler",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle, MaxCycles: 2},
		TaskList:  []roster.AgentTask{{Title: "cycle check", Scope: "cycle", Status: "done"}},
	})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	ent := cadence.Entry{ID: "sched-cycle", Target: cadence.TargetTool, Tool: "memory", Agent: "cycler"}
	ctx := kernelruntime.WithAgentProfile(context.Background(), p)
	if err := runScheduledTrackedTarget(ctx, k, "corr-cycle", ent, "run cycle tool", func(context.Context) (string, error) {
		return "ok", nil
	}); err != nil {
		t.Fatalf("runScheduledTrackedTarget: %v", err)
	}
	got, ok := k.Roster().Get("cycler")
	if !ok {
		t.Fatalf("cycler missing")
	}
	if got.Lifecycle.CompletedCycles != 1 {
		t.Fatalf("completed cycles = %d, want 1", got.Lifecycle.CompletedCycles)
	}
	if got.Retired {
		t.Fatalf("cycler retired too early: %+v", got)
	}
	if status := scheduledTaskStatus(got.TaskList, "cycle check"); status != "todo" {
		t.Fatalf("completed cycle task should reset to todo, got %q", status)
	}
}

func TestRunScheduledTrackedTargetDoesNotCompleteLifecycleOnFailure(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	p, err := k.AddProfile(roster.Profile{
		Slug:      "cycler-failed",
		Lifecycle: roster.AgentLifecycle{Mode: roster.LifecycleCycle, MaxCycles: 2},
	})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	ent := cadence.Entry{ID: "sched-cycle-failed", Target: cadence.TargetWorkflow, Workflow: "bad-flow", Agent: "cycler-failed"}
	ctx := kernelruntime.WithAgentProfile(context.Background(), p)
	if err := runScheduledTrackedTarget(ctx, k, "corr-cycle-failed", ent, "run bad workflow", func(context.Context) (string, error) {
		return "", errors.New("boom")
	}); err == nil {
		t.Fatalf("runScheduledTrackedTarget succeeded, want error")
	}
	got, ok := k.Roster().Get("cycler-failed")
	if !ok {
		t.Fatalf("cycler-failed missing")
	}
	if got.Lifecycle.CompletedCycles != 0 {
		t.Fatalf("failed lifecycle completed cycles = %d, want 0", got.Lifecycle.CompletedCycles)
	}
	if got.Retired {
		t.Fatalf("failed cycle should not retire agent: %+v", got)
	}
}

func TestRunScheduledTrackedTargetHonorsAgentToolPolicy(t *testing.T) {
	probe := &scheduledProbeTool{}
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
		Tools:    map[string]agent.Tool{"probe": probe},
		Edict:    edict.New(edict.Options{UnknownAllow: true}),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	p, err := k.AddProfile(roster.Profile{
		Slug:     "locked",
		ToolDeny: []string{"probe"},
	})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	ent := cadence.Entry{ID: "sched-probe", Target: cadence.TargetTool, Tool: "probe", Agent: "locked"}
	ctx := kernelruntime.WithAgentProfile(context.Background(), p)
	err = runScheduledTrackedTarget(ctx, k, "corr-probe", ent, "run denied tool", func(ctx context.Context) (string, error) {
		_, err := k.RunTool(ctx, "corr-probe", "schedule-sched-probe", "probe", json.RawMessage(`{}`))
		return "", err
	})
	if err == nil || !strings.Contains(err.Error(), "agent tool denylist") {
		t.Fatalf("scheduled tool err = %v, want agent tool denylist", err)
	}
	if got := probe.calls.Load(); got != 0 {
		t.Fatalf("denied scheduled tool invoked %d time(s), want 0", got)
	}
}

func scheduledTaskStatus(tasks []roster.AgentTask, title string) string {
	for _, task := range tasks {
		if task.Title == title {
			return task.Status
		}
	}
	return ""
}

func TestRunScheduledMemoryCleanPublishesSummary(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := runScheduledMemoryClean(context.Background(), k, "corr-memory-clean", "sched-memory-clean"); err != nil {
		t.Fatalf("runScheduledMemoryClean: %v", err)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == "corr-memory-clean" && e.Subject == "schedule.system_task.memory_clean" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("schedule memory_clean summary event not journaled")
	}
}

func TestRunScheduledMemoryTidyPublishesSummary(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := runScheduledMemoryTidy(context.Background(), k, "corr-memory-tidy", "sched-memory-tidy"); err != nil {
		t.Fatalf("runScheduledMemoryTidy: %v", err)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == "corr-memory-tidy" && e.Subject == "schedule.system_task.memory_tidy" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("schedule memory_tidy summary event not journaled")
	}
}

func TestRunScheduledLogCleanPublishesSummary(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := runScheduledLogClean(context.Background(), k, "corr-log-clean", "sched-log-clean"); err != nil {
		t.Fatalf("runScheduledLogClean: %v", err)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "corr-log-clean" || e.Subject != "schedule.system_task.log_clean" {
			return nil
		}
		found = true
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("payload unmarshal: %v", err)
		}
		if payload["schedule_id"] != "sched-log-clean" || payload["system_task"] != cadence.SystemTaskLogClean {
			t.Fatalf("payload missing schedule/system task fields: %v", payload)
		}
		if payload["effect_class"] != "log_maintenance" || payload["physical_deletion"] != false {
			t.Fatalf("payload missing safe log maintenance metadata: %v", payload)
		}
		return nil
	})
	if !found {
		t.Fatalf("schedule log_clean summary event not journaled")
	}
}

func TestRunScheduledGraveyardScanReportsOnly(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.AddProfile(roster.Profile{Slug: "dead", Soul: "x"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, err := k.SetProfileRetired("dead", true, "obsolete"); err != nil {
		t.Fatalf("SetProfileRetired: %v", err)
	}
	// Retention disabled (default keep-forever): reports graveyard size, flags none.
	t.Setenv("AGEZT_GRAVEYARD_RETENTION_DAYS", "")
	if err := runScheduledGraveyardScan(context.Background(), k, "corr-grave", "sched-grave"); err != nil {
		t.Fatalf("runScheduledGraveyardScan: %v", err)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "corr-grave" || e.Subject != "schedule.system_task.graveyard_scan" {
			return nil
		}
		found = true
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("payload unmarshal: %v", err)
		}
		if payload["system_task"] != cadence.SystemTaskGraveyardScan || payload["action"] != "report_only" {
			t.Fatalf("payload not a report-only graveyard scan: %v", payload)
		}
		if n, _ := payload["graveyard_count"].(float64); n != 1 {
			t.Fatalf("graveyard_count = %v, want 1", payload["graveyard_count"])
		}
		if n, _ := payload["eligible_count"].(float64); n != 0 {
			t.Fatalf("eligible_count = %v, want 0 with retention disabled", payload["eligible_count"])
		}
		return nil
	})
	if !found {
		t.Fatal("graveyard_scan summary event not journaled")
	}
	// Non-destructive: the retired agent still exists after the scan.
	if p, ok := k.Roster().Get("dead"); !ok || !p.Retired {
		t.Fatalf("graveyard scan must not remove the retired agent; got ok=%v retired=%v", ok, p.Retired)
	}
}

func TestRunScheduledArtifactCollectPublishesSummary(t *testing.T) {
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := runScheduledArtifactCollect(context.Background(), k, "corr-artifact-collect", "sched-artifact-collect"); err != nil {
		t.Fatalf("runScheduledArtifactCollect: %v", err)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID == "corr-artifact-collect" && e.Subject == "schedule.system_task.artifact_collect" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("schedule artifact_collect summary event not journaled")
	}
}

func TestRunScheduledCatalogSyncPublishesSummaryAndReloads(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(scheduleCatalogSyncFixture))
	}))
	defer ts.Close()
	t.Setenv("AGEZT_CATALOG_URL", ts.URL)

	var reloads atomic.Int32
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
		OnReload: func() error {
			reloads.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if err := runScheduledCatalogSync(context.Background(), k, "corr-catalog-sync", "sched-catalog-sync"); err != nil {
		t.Fatalf("runScheduledCatalogSync: %v", err)
	}
	if got := reloads.Load(); got != 1 {
		t.Fatalf("reloads = %d, want 1", got)
	}

	found := false
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.CorrelationID != "corr-catalog-sync" || e.Subject != "catalog.sync" || e.Kind != event.KindCatalogSynced {
			return nil
		}
		found = true
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("payload unmarshal: %v", err)
		}
		if payload["schedule_id"] != "sched-catalog-sync" || payload["system_task"] != cadence.SystemTaskCatalogSync {
			t.Fatalf("payload missing schedule/system task fields: %v", payload)
		}
		if payload["effect_class"] != "config_update" {
			t.Fatalf("payload missing effect class: %v", payload)
		}
		if payload["provider_count"] != float64(1) || payload["model_count"] != float64(1) {
			t.Fatalf("payload counts = providers %v models %v, want 1/1", payload["provider_count"], payload["model_count"])
		}
		return nil
	})
	if !found {
		t.Fatalf("schedule catalog_sync summary event not journaled")
	}
}
