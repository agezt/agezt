// SPDX-License-Identifier: MIT

// Cadence/scheduled-task runner + payload type + scheduling helpers extracted
// from main_cadence.go during Day 211 god-file refactor (#52).
// Public API unchanged.
package main

import (
	"context"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func runScheduledTrackedTarget(ctx context.Context, k *kernelruntime.Kernel, corr string, ent cadence.Entry, intent string, run func(context.Context) (string, error)) error {
	sf := schedulePayloadForEntry(ent, intent)
	action := controlplaneScheduleAction(sf)
	receivedPayload := map[string]any{
		"schedule_id":      ent.ID,
		"intent":           action,
		"scheduled_intent": intent,
		"target":           ent.Target,
		"agent":            ent.Agent,
		"workflow":         ent.Workflow,
		"system_task":      ent.SystemTask,
		"tool":             ent.Tool,
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskReceived,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload:       receivedPayload,
	})

	answer, err := run(ctx)
	if err != nil {
		_, _ = k.Bus().Publish(event.Spec{
			Subject:       "schedule.task",
			Kind:          event.KindTaskFailed,
			Actor:         "schedule",
			CorrelationID: corr,
			Payload: map[string]any{
				"schedule_id": ent.ID,
				"target":      ent.Target,
				"reason":      "error",
				"error":       err.Error(),
			},
		})
		return err
	}
	k.CompleteAgentLifecycle(ctx, corr)
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "schedule.task",
		Kind:          event.KindTaskCompleted,
		Actor:         "schedule",
		CorrelationID: corr,
		Payload: map[string]any{
			"schedule_id": ent.ID,
			"target":      ent.Target,
			"answer":      truncateScheduledAnswer(answer),
			"iters":       0,
		},
	})
	return nil
}
type scheduledTargetPayload struct {
	ScheduleID string
	Intent     string
	Target     string
	Agent      string
	Workflow   string
	SystemTask string
	Tool       string
}
func schedulePayloadForEntry(ent cadence.Entry, intent string) scheduledTargetPayload {
	return scheduledTargetPayload{
		ScheduleID: ent.ID,
		Intent:     intent,
		Target:     ent.Target,
		Agent:      ent.Agent,
		Workflow:   ent.Workflow,
		SystemTask: ent.SystemTask,
		Tool:       ent.Tool,
	}
}
func controlplaneScheduleAction(p scheduledTargetPayload) string {
	switch p.Target {
	case "workflow":
		if p.Workflow != "" {
			return "run workflow " + p.Workflow
		}
	case "system_task":
		if p.SystemTask != "" {
			return "run system task " + p.SystemTask
		}
	case "tool":
		if p.Tool != "" {
			return "run tool " + p.Tool
		}
	}
	if p.Agent != "" && p.Intent != "" {
		return "wake " + p.Agent + ": " + p.Intent
	}
	if p.Intent != "" {
		return p.Intent
	}
	return p.ScheduleID
}
func truncateScheduledAnswer(s string) string {
	const max = 4096
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
