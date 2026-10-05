package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard dispatch helpers
//             (buildWorkboardDispatchIntent, publishWorkboardDispatch,
//             latestWorkboardRunID, workboardWatchEvents). Extracted from
//             workboard_dispatch.go during Day 211 god-file refactor (#70). Public
//             API unchanged.

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workboard"
)

func buildWorkboardDispatchIntent(explicit string, task workboard.Task) string {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return explicit
	}
	var b strings.Builder
	b.WriteString("Workboard task dispatch.\n")
	b.WriteString("You are assigned a durable AGEZT workboard task. Use the workboard tool to heartbeat, comment, block, link artifacts/runs, and complete the task when it is actually done.\n")
	b.WriteString("Task ID: ")
	b.WriteString(task.ID)
	b.WriteString("\nTitle: ")
	b.WriteString(task.Title)
	b.WriteString("\nStatus: ")
	b.WriteString(string(task.Status))
	if task.Priority != 0 {
		b.WriteString("\nPriority: ")
		b.WriteString(fmt.Sprintf("%d", task.Priority))
	}
	if task.Tenant != "" {
		b.WriteString("\nTenant: ")
		b.WriteString(task.Tenant)
	}
	if task.Description != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(task.Description)
	}
	if len(task.Tags) > 0 {
		b.WriteString("\nTags: ")
		b.WriteString(strings.Join(task.Tags, ", "))
	}
	b.WriteString("\nExpected finish: call workboard {\"op\":\"complete\",\"id\":\"")
	b.WriteString(task.ID)
	b.WriteString("\"} only when complete; otherwise call workboard block/comment with the concrete reason or next step.")
	return b.String()
}
func publishWorkboardDispatch(k *kernelruntime.Kernel, corr string, task workboard.Task, phase, agent, reason, answer, errText string) {
	if k == nil || k.Bus() == nil {
		return
	}
	payload := map[string]any{
		"phase":          phase,
		"id":             task.ID,
		"title":          task.Title,
		"status":         string(task.Status),
		"agent":          agent,
		"reason":         reason,
		"correlation_id": corr,
	}
	if answer != "" {
		payload["answer"] = answer
	}
	if errText != "" {
		payload["error"] = errText
	}
	if task.Seat != "" {
		payload["seat"] = task.Seat
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "workboard." + task.ID,
		Kind:          event.KindWorkboardTaskDispatched,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       payload,
	})
}
