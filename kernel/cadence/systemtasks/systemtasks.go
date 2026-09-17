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
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
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
