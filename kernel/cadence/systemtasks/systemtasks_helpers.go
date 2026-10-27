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
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/catalogapi"
	"github.com/agezt/agezt/kernel/contract/journalapi"
	"github.com/agezt/agezt/kernel/event"
)

// errNoCatalogLibrary is the answer the scheduled sync gives when the daemon
// holds no catalog library.
//
// The catalog directory and its syncer are the DAEMON's (W3.51): the kernel
// holds the contract's ports, and a host that was given none holds nil ports.
// The refusal is the answer — not a panic, and not a silent "nothing to do".
var errNoCatalogLibrary = errors.New("systemtasks: the catalog library is not available on this daemon")

// publishSummary journals a task's summary event.
//
// The publish error is DISCARDED, the way it always was: a summary event
// that could not be written must not fail the task it summarises. A daemon
// with no publisher at all is a different thing — there is nowhere to write
// to — and the task refuses rather than pretending the summary was recorded.
func publishSummary(ports cadenceapi.SystemTaskPorts, scheduleID, subject string, kind event.Kind, corr string, payload map[string]any) error {
	if ports.Publish == nil {
		return fmt.Errorf("schedule %s: the event bus is unavailable", scheduleID)
	}
	_ = ports.Publish.Publish(event.Spec{
		Subject:       subject,
		Kind:          kind,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       payload,
	})
	return nil
}

// runScheduledProfileDistill synthesizes the operator profile from accumulated
// memory (M1000). LLM-backed (unlike the maintenance tasks), so it runs at a low
// daily cadence; a no-op until there's enough accumulated memory to learn from.
func runScheduledProfileDistill(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	if ports.Memory == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	if ports.DistillProfile == nil {
		return fmt.Errorf("schedule %s: profile distillation unavailable", scheduleID)
	}
	report, err := ports.DistillProfile(ctx, corr)
	if err != nil {
		return err
	}
	return publishSummary(ports, scheduleID, "schedule.system_task.profile_distill", event.KindInfo, corr, map[string]any{
		"schedule_id":    scheduleID,
		"system_task":    cadence.SystemTaskProfileDistill,
		"input_records":  report.InputRecords,
		"facets_written": report.FacetsWritten,
		"facets":         report.Facets,
	})
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

func runScheduledGraveyardScan(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	_ = ctx
	retentionDays := graveyardRetentionDays()
	nowMS := time.Now().UnixMilli()
	cutoffMS := int64(0)
	if retentionDays > 0 {
		cutoffMS = nowMS - int64(retentionDays)*24*3600*1000
	}
	if ports.RosterList == nil {
		return fmt.Errorf("schedule %s: roster unavailable", scheduleID)
	}
	graveyard := 0
	eligible := make([]string, 0)
	for _, p := range ports.RosterList() {
		if !p.Retired {
			continue
		}
		graveyard++
		if cutoffMS > 0 && p.RetiredMS > 0 && p.RetiredMS <= cutoffMS {
			eligible = append(eligible, p.Slug)
		}
	}
	sort.Strings(eligible)
	return publishSummary(ports, scheduleID, "schedule.system_task.graveyard_scan", event.KindInfo, corr, map[string]any{
		"schedule_id":     scheduleID,
		"system_task":     cadence.SystemTaskGraveyardScan,
		"retention_days":  retentionDays,
		"graveyard_count": graveyard,
		"eligible_count":  len(eligible),
		"eligible":        eligible,
		// Explicit: this task only reports — it performs no removal.
		"action": "report_only",
	})
}

func runScheduledArtifactCollect(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	_ = ctx
	if ports.CollectArtifacts == nil {
		return fmt.Errorf("schedule %s: artifact index unavailable", scheduleID)
	}
	const olderThanDays = 30
	cutoff := time.Now().Add(-time.Duration(olderThanDays) * 24 * time.Hour).UnixMilli()
	collected, bytes := ports.CollectArtifacts(cutoff)
	return publishSummary(ports, scheduleID, "schedule.system_task.artifact_collect", event.KindInfo, corr, map[string]any{
		"schedule_id":     scheduleID,
		"system_task":     cadence.SystemTaskArtifactCollect,
		"older_than_days": olderThanDays,
		"cutoff_ms":       cutoff,
		"collected":       collected,
		"bytes":           bytes,
	})
}

func runScheduledMemoryClean(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	_ = ctx
	if ports.Memory == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	report, err := ports.Memory.CleanLowValue(corr, false)
	if err != nil {
		return err
	}
	return publishSummary(ports, scheduleID, "schedule.system_task.memory_clean", event.KindInfo, corr, map[string]any{
		"schedule_id": scheduleID,
		"system_task": cadence.SystemTaskMemoryClean,
		"scanned":     report.Scanned,
		"rejected":    report.Rejected,
		"removed":     report.Removed,
	})
}

func runScheduledMemoryTidy(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	_ = ctx
	if ports.Memory == nil {
		return fmt.Errorf("schedule %s: memory unavailable", scheduleID)
	}
	collapsed, err := ports.Memory.DedupeDistilled(corr, false)
	if err != nil {
		return err
	}
	return publishSummary(ports, scheduleID, "schedule.system_task.memory_tidy", event.KindInfo, corr, map[string]any{
		"schedule_id": scheduleID,
		"system_task": cadence.SystemTaskMemoryTidy,
		"collapsed":   collapsed,
	})
}

func runScheduledLogClean(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	_ = ctx
	if ports.ScanJournal == nil {
		return fmt.Errorf("schedule %s: journal unavailable", scheduleID)
	}
	var events int64
	var oldestMS int64
	var latestMS int64
	if err := ports.ScanJournal.Scan(func(r journalapi.Record) error {
		events++
		if oldestMS == 0 || (r.TSUnixMS > 0 && r.TSUnixMS < oldestMS) {
			oldestMS = r.TSUnixMS
		}
		if r.TSUnixMS > latestMS {
			latestMS = r.TSUnixMS
		}
		return nil
	}); err != nil {
		return err
	}
	headSeq, headHash := ports.ScanJournal.Head()
	return publishSummary(ports, scheduleID, "schedule.system_task.log_clean", event.KindInfo, corr, map[string]any{
		"schedule_id":       scheduleID,
		"system_task":       cadence.SystemTaskLogClean,
		"events_scanned":    events,
		"oldest_unix_ms":    oldestMS,
		"latest_unix_ms":    latestMS,
		"head_seq":          headSeq,
		"head_hash":         headHash,
		"physical_deletion": false,
		"effect_class":      "log_maintenance",
	})
}

func runScheduledCatalogSync(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID string) error {
	url := envOrDefaultLocal(brand.EnvPrefix+"CATALOG_URL", catalogapi.DefaultSyncURL)
	// The syncer is the DAEMON's (W3.51): the kernel holds the contract's
	// port, and this module may not name the catalog module. A host with no
	// syncer has nothing to sync from — the refusal is the answer.
	syncer := ports.CatalogSyncer
	if syncer == nil {
		return errNoCatalogLibrary
	}
	raw, cat, res, err := syncer.Sync(ctx, url, 0)
	if err != nil {
		_ = publishSummary(ports, scheduleID, "catalog.sync", event.KindCatalogSyncFailed, corr, map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": err.Error()})
		return err
	}
	if ports.CatalogStore == nil {
		return errNoCatalogLibrary
	}
	if err := ports.CatalogStore.SaveAPI(raw, url); err != nil {
		_ = publishSummary(ports, scheduleID, "catalog.sync", event.KindCatalogSyncFailed, corr, map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": "save: " + err.Error()})
		return fmt.Errorf("save: %w", err)
	}
	if ports.Reload == nil {
		return fmt.Errorf("schedule %s: catalog reload unavailable", scheduleID)
	}
	freshCat, providersReloaded, provErr := ports.Reload()
	if freshCat == nil {
		// The reload failed. The failure is the provider registry's or the
		// catalog's, and the message says which — but a reload that answers
		// no catalog and NO error at all must not take the process down: the
		// message is built from what the port actually answered.
		errText := "the catalog could not be reloaded"
		if provErr != nil {
			errText = provErr.Error()
		}
		_ = publishSummary(ports, scheduleID, "catalog.sync", event.KindCatalogSyncFailed, corr, map[string]any{"url": url, "schedule_id": scheduleID, "system_task": cadence.SystemTaskCatalogSync, "error": errText})
		if provErr != nil {
			return fmt.Errorf("reload: %w", provErr)
		}
		return fmt.Errorf("reload: %s", errText)
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
	_ = publishSummary(ports, scheduleID, "catalog.sync", event.KindCatalogSynced, corr, payload)
	_ = cat
	return nil
}

func envOrDefaultLocal(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
