// SPDX-License-Identifier: MIT

// Package systemtasks holds the executors behind cadence's built-in system
// tasks (catalog_sync, artifact_collect, memory_clean, memory_tidy, log_clean,
// graveyard_scan, profile_distill) — the daemon-side maintenance work a
// schedule entry with Target=system_task dispatches (Phase 2.6 extraction from
// cmd/agezt; the catalogue + validation already lived in kernel/cadence).
//
// IMPORT-CYCLE CONSTRAINT: systemtasks imports kernel/runtime (the executors
// run against a live kernel). Nothing in kernel/cadence or kernel/runtime may
// EVER import systemtasks — the dependency arrow points one way only
// (cadence -> catalogue metadata; systemtasks -> cadence + runtime; the daemon
// wires the two together at its buildCadence dispatch site).
package systemtasks

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// Info returns the catalogue metadata for a system task by name (executor,
// category, effect class, LLM use) — the schedule.fired payload decoration.
func Info(name string) (cadence.SystemTaskInfo, bool) {
	name = strings.TrimSpace(name)
	for _, info := range cadence.SystemTaskInfos() {
		if info.Name == name {
			return info, true
		}
	}
	return cadence.SystemTaskInfo{}, false
}

// Run dispatches one system-task firing to its executor. task is the
// cadence.SystemTask* name from the schedule entry; unknown names error.
func Run(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID, task string) error {
	switch strings.TrimSpace(task) {
	case cadence.SystemTaskCatalogSync:
		return runScheduledCatalogSync(ctx, k, corr, scheduleID)
	case cadence.SystemTaskArtifactCollect:
		return runScheduledArtifactCollect(ctx, k, corr, scheduleID)
	case cadence.SystemTaskMemoryClean:
		return runScheduledMemoryClean(ctx, k, corr, scheduleID)
	case cadence.SystemTaskMemoryTidy:
		return runScheduledMemoryTidy(ctx, k, corr, scheduleID)
	case cadence.SystemTaskLogClean:
		return runScheduledLogClean(ctx, k, corr, scheduleID)
	case cadence.SystemTaskGraveyardScan:
		return runScheduledGraveyardScan(ctx, k, corr, scheduleID)
	case cadence.SystemTaskProfileDistill:
		return runScheduledProfileDistill(ctx, k, corr, scheduleID)
	default:
		return fmt.Errorf("schedule %s: unknown system task %q", scheduleID, task)
	}
}

// runScheduledProfileDistill synthesizes the operator profile from accumulated
// memory (M1000). LLM-backed (unlike the maintenance tasks), so it runs at a low
// daily cadence; a no-op until there's enough accumulated memory to learn from.
func runScheduledProfileDistill(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	if k.Memory() == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	report, err := k.DistillProfile(ctx, corr)
	if err != nil {
		return err
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.profile_distill",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":    scheduleID,
			"system_task":    cadence.SystemTaskProfileDistill,
			"input_records":  report.InputRecords,
			"facets_written": report.FacetsWritten,
			"facets":         report.Facets,
		},
	})
	return nil
}

// graveyardRetentionDays is the retention window for the graveyard scan: retired
// agents older than this are reported as removal-eligible. 0 (the default) means
// keep-forever — the scan still reports graveyard size but flags nothing eligible.
// This task is NOTIFY-ONLY: it never archives or deletes (removal stays an explicit
// operator action), so a misconfigured window can only over-report, never destroy.
func graveyardRetentionDays() int {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "GRAVEYARD_RETENTION_DAYS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func runScheduledGraveyardScan(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	retentionDays := graveyardRetentionDays()
	nowMS := time.Now().UnixMilli()
	cutoffMS := int64(0)
	if retentionDays > 0 {
		cutoffMS = nowMS - int64(retentionDays)*24*3600*1000
	}
	graveyard := 0
	eligible := make([]string, 0)
	for _, p := range k.Roster().List() {
		if !p.Retired {
			continue
		}
		graveyard++
		if cutoffMS > 0 && p.RetiredMS > 0 && p.RetiredMS <= cutoffMS {
			eligible = append(eligible, p.Slug)
		}
	}
	sort.Strings(eligible)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.graveyard_scan",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":     scheduleID,
			"system_task":     cadence.SystemTaskGraveyardScan,
			"retention_days":  retentionDays,
			"graveyard_count": graveyard,
			"eligible_count":  len(eligible),
			"eligible":        eligible,
			// Explicit: this task only reports — it performs no removal.
			"action": "report_only",
		},
	})
	return nil
}

func runScheduledArtifactCollect(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	idx := k.ArtifactIndex()
	if idx == nil {
		return fmt.Errorf("schedule %s: artifact index unavailable", scheduleID)
	}
	const olderThanDays = 30
	cutoff := time.Now().Add(-time.Duration(olderThanDays) * 24 * time.Hour).UnixMilli()
	collected, bytes := idx.Collect(cutoff)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.artifact_collect",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":     scheduleID,
			"system_task":     cadence.SystemTaskArtifactCollect,
			"older_than_days": olderThanDays,
			"cutoff_ms":       cutoff,
			"collected":       collected,
			"bytes":           bytes,
		},
	})
	return nil
}

func runScheduledMemoryClean(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	if k.Memory() == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	report, err := k.Memory().CleanLowValue(corr, false)
	if err != nil {
		return err
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.memory_clean",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": scheduleID,
			"system_task": cadence.SystemTaskMemoryClean,
			"scanned":     report.Scanned,
			"rejected":    report.Rejected,
			"removed":     report.Removed,
		},
	})
	return nil
}

func runScheduledMemoryTidy(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	if k.Memory() == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	collapsed, err := k.Memory().DedupeDistilled(corr, false)
	if err != nil {
		return err
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.memory_tidy",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": scheduleID,
			"system_task": cadence.SystemTaskMemoryTidy,
			"collapsed":   collapsed,
		},
	})
	return nil
}

func runScheduledLogClean(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	_ = ctx
	j := k.Journal()
	if j == nil {
		return fmt.Errorf("schedule %s: journal unavailable", scheduleID)
	}
	var events int64
	var oldestMS int64
	var latestMS int64
	if err := j.Range(func(e *event.Event) error {
		events++
		if oldestMS == 0 || (e.TSUnixMS > 0 && e.TSUnixMS < oldestMS) {
			oldestMS = e.TSUnixMS
		}
		if e.TSUnixMS > latestMS {
			latestMS = e.TSUnixMS
		}
		return nil
	}); err != nil {
		return err
	}
	headSeq, headHash := j.Head()
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.system_task.log_clean",
		Kind:          event.KindInfo,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id":       scheduleID,
			"system_task":       cadence.SystemTaskLogClean,
			"events_scanned":    events,
			"oldest_unix_ms":    oldestMS,
			"latest_unix_ms":    latestMS,
			"head_seq":          headSeq,
			"head_hash":         headHash,
			"physical_deletion": false,
			"effect_class":      "log_maintenance",
		},
	})
	return nil
}

func runScheduledCatalogSync(ctx context.Context, k *kernelruntime.Kernel, corr, scheduleID string) error {
	url := envOrDefaultLocal(brand.EnvPrefix+"CATALOG_URL", catalog.DefaultSyncURL)
	syncer := catalog.NewSyncer()
	syncer.URL = url
	raw, cat, res, err := syncer.Sync(ctx)
	if err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload:       map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": err.Error()},
		})
		return err
	}
	if err := k.CatalogStore().SaveAPI(raw, url); err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload:       map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": "save: " + err.Error()},
		})
		return fmt.Errorf("save: %w", err)
	}
	freshCat, providersReloaded, provErr := k.Reload()
	if freshCat == nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload:       map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": "reload: " + provErr.Error()},
		})
		return fmt.Errorf("reload: %w", provErr)
	}
	payload := map[string]any{
		"url":                url,
		"schedule_id":        scheduleID,
		"system_task":        cadence.SystemTaskCatalogSync,
		"bytes":              res.Bytes,
		"provider_count":     res.ProviderCount,
		"model_count":        res.ModelCount,
		"duration_ms":        res.Duration.Milliseconds(),
		"providers_reloaded": providersReloaded,
		"effect_class":       "config_update",
	}
	if provErr != nil {
		payload["provider_reload_error"] = provErr.Error()
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "catalog.sync",
		Kind:          event.KindCatalogSynced,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       payload,
	})
	_ = cat
	return nil
}

func envOrDefaultLocal(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
