package controlplane

// Provenance: SPDX-License-Identifier: MIT Workboard dispatch helpers
//             (buildWorkboardDispatchIntent, publishWorkboardDispatch,
//             latestWorkboardRunID, workboardWatchEvents). Extracted from
//             workboard_dispatch.go during Day 211 god-file refactor (#70). Public
//             API unchanged.

import (
	"github.com/agezt/agezt/kernel/app/binding"
	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
)

func publishWorkboardDispatch(k any, corr string, task workboardapi.Task, phase, agent, reason, answer, errText string) {
	ports := binding.Fill(k, appeventbus.PortsFromKernel)
	if !ports.Ready() {
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
	_, _ = ports.Publish(event.Spec{
		Subject:       "workboard." + task.ID,
		Kind:          event.KindWorkboardTaskDispatched,
		Actor:         "workboard",
		CorrelationID: corr,
		Payload:       payload,
	})
}
