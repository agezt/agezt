// SPDX-License-Identifier: MIT

package systemtasks

// Provenance: systemtasks_helpers.go: 7 runScheduled* tasks + graveyardRetentionDays
//             + envOrDefaultLocal split off from systemtasks.go during the Day 211
//             god-file refactor (#131). Public API unchanged.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// errNoCatalogLibrary is the answer the scheduled sync gives when the daemon
// holds no catalog library.
//
// The catalog directory and its syncer are the DAEMON's (W3.51): the kernel
// holds the contract's ports, and a host that was given none holds nil ports.
// The refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoCatalogLibrary = errors.New("systemtasks: the catalog library is not available on this daemon")

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
	url := envOrDefaultLocal(brand.EnvPrefix+"CATALOG_URL", catalogapi.DefaultSyncURL)
	// The syncer is the DAEMON's (W3.51): the kernel holds the contract's
	// port, and this module may not name the catalog module. A host with no
	// syncer has nothing to sync from — the refusal is the answer.
	syncer := k.CatalogSyncer()
	if syncer == nil {
		return errNoCatalogLibrary
	}
	raw, cat, res, err := syncer.Sync(ctx, url, 0)
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
	if k.CatalogStore() == nil {
		return errNoCatalogLibrary
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
