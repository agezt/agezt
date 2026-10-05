package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard dispatch helpers
//             (buildWorkboardDispatchIntent, publishWorkboardDispatch,
//             latestWorkboardRunID, workboardWatchEvents). Extracted from
//             workboard_dispatch.go during Day 211 god-file refactor (#70). Public
//             API unchanged.

import (
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workboard"
)

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
