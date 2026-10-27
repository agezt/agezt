// SPDX-License-Identifier: MIT

package systemtasks

// Package documentation lives in doc.go.

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
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
//
// The ports are the daemon's capabilities (cadenceapi.SystemTaskPorts): the
// tasks ask for what they need through the contract and never name the
// kernel. A capability the daemon does not have is a nil field, and the task
// that needs it refuses — it does not report a clean store it never looked
// at.
func Run(ctx context.Context, ports cadenceapi.SystemTaskPorts, corr, scheduleID, task string) error {
	switch strings.TrimSpace(task) {
	case cadence.SystemTaskCatalogSync:
		return runScheduledCatalogSync(ctx, ports, corr, scheduleID)
	case cadence.SystemTaskArtifactCollect:
		return runScheduledArtifactCollect(ctx, ports, corr, scheduleID)
	case cadence.SystemTaskMemoryClean:
		return runScheduledMemoryClean(ctx, ports, corr, scheduleID)
	case cadence.SystemTaskMemoryTidy:
		return runScheduledMemoryTidy(ctx, ports, corr, scheduleID)
	case cadence.SystemTaskLogClean:
		return runScheduledLogClean(ctx, ports, corr, scheduleID)
	case cadence.SystemTaskGraveyardScan:
		return runScheduledGraveyardScan(ctx, ports, corr, scheduleID)
	case cadence.SystemTaskProfileDistill:
		return runScheduledProfileDistill(ctx, ports, corr, scheduleID)
	default:
		return fmt.Errorf("schedule %s: unknown system task %q", scheduleID, task)
	}
}
