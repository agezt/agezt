// SPDX-License-Identifier: MIT

package systemtasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

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
