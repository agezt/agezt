// SPDX-License-Identifier: MIT

package systemtasks

// Package documentation lives in doc.go.

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
