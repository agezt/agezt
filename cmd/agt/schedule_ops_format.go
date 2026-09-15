// SPDX-License-Identifier: MIT
//
// cmd/agt schedule text-formatter helpers (scheduleTargetStatusText,
// scheduleExecutorText, scheduleActionText). Extracted from schedule_ops.go
// during Day 211 god-file refactor (#58). Public API unchanged.
package main

import (
	"strings"

	"github.com/agezt/agezt/kernel/cadence"
)

func scheduleTargetStatusText(m map[string]any) string {
	status, _ := m["target_status"].(string)
	errText, _ := m["target_error"].(string)
	status = strings.TrimSpace(status)
	errText = strings.TrimSpace(errText)
	if strings.EqualFold(status, "blocked") || errText != "" {
		if errText == "" {
			return "blocked"
		}
		return "blocked (" + errText + ")"
	}
	if strings.EqualFold(status, "ready") {
		return "ready"
	}
	return ""
}
func scheduleExecutorText(executor string, usesLLM bool, known bool) string {
	if executor == "" {
		return ""
	}
	if !known {
		return executor
	}
	if usesLLM {
		return executor + "/llm"
	}
	return executor + "/no-llm"
}
func scheduleActionText(m map[string]string) string {
	switch m["target"] {
	case cadence.TargetWorkflow:
		if m["workflow"] != "" {
			return "run workflow " + m["workflow"]
		}
	case cadence.TargetSystemTask:
		if m["system_task"] != "" {
			return "run system task " + m["system_task"]
		}
	case cadence.TargetTool:
		if m["tool"] != "" {
			return "run tool " + m["tool"]
		}
	}
	if m["agent"] != "" && m["intent"] != "" {
		return "wake " + m["agent"] + ": " + m["intent"]
	}
	if m["intent"] != "" {
		return m["intent"]
	}
	return m["id"]
}
