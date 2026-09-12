// SPDX-License-Identifier: MIT

// Agent activity misc text: agentRetryPolicySummary + pausedTriggerSummary + removalCleanupSummary.
// Code extracted from roster_activity_text.go during the Day-77 god-file split. Public API unchanged.
package controlplane


import (
	"strconv"
	"strings"
)


func agentRetryPolicySummary(pl map[string]any) string {
	var bits []string
	if delay := plInt(pl, "delay_ms"); delay > 0 {
		bits = append(bits, "delay "+strconv.Itoa(delay)+"ms")
	}
	if backoff := strings.TrimSpace(plString(pl, "backoff")); backoff != "" {
		bits = append(bits, "backoff "+backoff)
	}
	if retryOn := plStrings(pl, "retry_on"); len(retryOn) > 0 {
		bits = append(bits, "retry_on "+strings.Join(retryOn, ","))
	}
	if len(bits) == 0 {
		return ""
	}
	return " (" + strings.Join(bits, "; ") + ")"
}

func pausedTriggerSummary(pl map[string]any) string {
	var bits []string
	if n := plInt(pl, "standing_paused"); n > 0 {
		bits = append(bits, strconv.Itoa(n)+" standing paused")
	}
	if n := plInt(pl, "schedules_paused"); n > 0 {
		bits = append(bits, strconv.Itoa(n)+" schedules paused")
	}
	return strings.Join(bits, ", ")
}

func removalCleanupSummary(pl map[string]any) string {
	var bits []string
	fields := []struct {
		key   string
		label string
	}{
		{"standing_removed", "standing removed"},
		{"schedules_removed", "schedules removed"},
		{"memories_forgotten", "private memories forgotten"},
		{"authored_memories_forgotten", "authored memories forgotten"},
		{"skills_archived", "skills archived"},
		{"configs_deleted", "configs deleted"},
		{"configs_access_pruned", "shared config access pruned"},
		{"workspaces_deleted", "workspaces deleted"},
		{"subagents_retired", "sub-agents retired"},
		{"mailbox_messages_retained", "mailbox/audit messages retained"},
		{"workflow_refs_retained", "workflow refs retained"},
		{"subagent_workflow_refs_retained", "sub-agent workflow refs retained"},
	}
	for _, f := range fields {
		if n := plInt(pl, f.key); n > 0 {
			bits = append(bits, strconv.Itoa(n)+" "+f.label)
		}
	}
	return strings.Join(bits, ", ")
}
