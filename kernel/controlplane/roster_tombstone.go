// SPDX-License-Identifier: MIT

package controlplane

// Agent tombstone handler (M846): the "what was the last known
// state of an agent that has since been removed" read-only endpoint. Carved
// out of roster.go during the Day 24 god file split #4 so the main file can
// focus on the live lifecycle path.

import (
	"net"
	"strings"
)

func (s *Server) handleAgentTombstone(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	impact := s.agentImpactResult(p)
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	tombstone := map[string]any{
		"slug":             p.Slug,
		"name":             p.Name,
		"kind":             p.Kind(),
		"system":           p.System,
		"description":      p.Description,
		"manager":          manager,
		"retired":          p.Retired,
		"retired_ms":       p.RetiredMS,
		"retired_reason":   p.RetiredReason,
		"lifecycle_mode":   strings.TrimSpace(p.Lifecycle.Mode),
		"completed_cycles": p.Lifecycle.CompletedCycles,
		"max_cycles":       p.Lifecycle.MaxCycles,
		"memory_scope":     strings.TrimSpace(p.MemoryScope),
		"model":            strings.TrimSpace(p.Model),
		// Durable footprint left behind — the counts the removal cascade would act on.
		"footprint": map[string]any{
			"standing_orders":  impact["standing_count"],
			"schedules":        impact["schedule_count"],
			"memories":         impact["memory_count"],
			"authored_shared":  impact["authored_shared_memory_count"],
			"skills":           impact["skill_count"],
			"configs":          impact["config_count"],
			"workspaces":       impact["workspace_count"],
			"workflow_refs":    impact["workflow_ref_count"],
			"mailbox_messages": impact["mailbox_message_count"],
			"subagents":        impact["subagent_count"],
		},
		// Mailbox/audit messages and workflow refs are retained by design, not
		// deleted, so the tombstone records them as the agent's lasting trace.
		"retained_by_design": map[string]any{
			"mailbox_messages": impact["mailbox_message_count"],
			"workflow_refs":    impact["workflow_ref_count"],
		},
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tombstone": tombstone}})
}
