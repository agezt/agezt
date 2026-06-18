// SPDX-License-Identifier: MIT

package controlplane

// Reaper on-demand scan (#53, M903) — the read-only detail behind the pulse
// ReaperObserver and a future `agt reaper` / UI surface. Detection only; the
// operator still retires (graveyard) or collects.

import (
	"net"
	"time"
)

func (s *Server) handleReaperScan(conn net.Conn, req Request) {
	idleDays := intArg(req.Args["idle_days"], 30)
	staleDays := intArg(req.Args["stale_days"], 30)
	now := time.Now()
	agentCut := now.Add(-time.Duration(idleDays) * 24 * time.Hour).UnixMilli()
	artifactCut := now.Add(-time.Duration(staleDays) * 24 * time.Hour).UnixMilli()

	rep := s.k.ReaperScan(agentCut, artifactCut)

	agents := make([]map[string]any, 0, len(rep.DeadAgents))
	for _, a := range rep.DeadAgents {
		agents = append(agents, map[string]any{
			"slug": a.Slug, "name": a.Name, "last_active_ms": a.LastActiveMS,
		})
	}
	degraded := make([]map[string]any, 0, len(rep.DegradedAgents))
	for _, a := range rep.DegradedAgents {
		degraded = append(degraded, map[string]any{
			"slug": a.Slug, "name": a.Name, "failures": a.Failures, "window": a.Window,
			"threshold": a.Threshold, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_failure_ms": a.LastFailureMS, "last_reason": a.LastReason,
		})
	}
	misconfigured := make([]map[string]any, 0, len(rep.MisconfiguredAgents))
	for _, a := range rep.MisconfiguredAgents {
		misconfigured = append(misconfigured, map[string]any{
			"slug": a.Slug, "name": a.Name, "issues": a.Issues, "doctor_agent": a.DoctorAgent,
			"self_repair_enabled": a.SelfRepairEnabled, "escalate_to": a.EscalateTo,
		})
	}
	retryPressure := make([]map[string]any, 0, len(rep.RetryPressure))
	for _, a := range rep.RetryPressure {
		retryPressure = append(retryPressure, map[string]any{
			"slug": a.Slug, "name": a.Name, "count": a.Count, "threshold": a.Threshold,
			"window_sec": a.WindowSec, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_retry_ms": a.LastRetryMS, "last_reason": a.LastReason,
			"next_attempt": a.NextAttempt, "max_attempts": a.MaxAttempts,
		})
	}
	routing := make([]map[string]any, 0, len(rep.RoutingPressure))
	for _, a := range rep.RoutingPressure {
		routing = append(routing, map[string]any{
			"slug": a.Slug, "name": a.Name, "count": a.Count, "threshold": a.Threshold,
			"window_sec": a.WindowSec, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_fallback_ms": a.LastFallbackMS, "last_reason": a.LastReason,
			"last_failed_model": a.LastFailedModel, "last_next_model": a.LastNextModel, "task_type": a.TaskType,
		})
	}
	forced := make([]map[string]any, 0, len(rep.RoutingForced))
	for _, a := range rep.RoutingForced {
		forced = append(forced, map[string]any{
			"slug": a.Slug, "name": a.Name, "count": a.Count, "threshold": a.Threshold,
			"window_sec": a.WindowSec, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_fallback_ms": a.LastFallbackMS, "last_forced_ms": a.LastForcedMS,
			"last_reason": a.LastReason, "task_type": a.TaskType, "forced_chain": a.ForcedChain, "routing_force_generation": a.ForceGeneration,
		})
	}
	forcedFailed := make([]map[string]any, 0, len(rep.RoutingForcedFailed))
	for _, a := range rep.RoutingForcedFailed {
		forcedFailed = append(forcedFailed, map[string]any{
			"slug": a.Slug, "name": a.Name, "count": a.Count, "threshold": a.Threshold,
			"window_sec": a.WindowSec, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_fallback_ms": a.LastFallbackMS, "last_forced_ms": a.LastForcedMS,
			"last_reason": a.LastReason, "task_type": a.TaskType, "forced_chain": a.ForcedChain, "routing_force_generation": a.ForceGeneration,
		})
	}
	forcedExhausted := make([]map[string]any, 0, len(rep.RoutingForcedExhausted))
	for _, a := range rep.RoutingForcedExhausted {
		forcedExhausted = append(forcedExhausted, map[string]any{
			"slug": a.Slug, "name": a.Name, "count": a.Count, "threshold": a.Threshold,
			"window_sec": a.WindowSec, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_fallback_ms": a.LastFallbackMS, "last_forced_ms": a.LastForcedMS,
			"last_reason": a.LastReason, "task_type": a.TaskType, "forced_chain": a.ForcedChain, "routing_force_generation": a.ForceGeneration,
		})
	}
	unstable := make([]map[string]any, 0, len(rep.RoutingUnstable))
	for _, a := range rep.RoutingUnstable {
		unstable = append(unstable, map[string]any{
			"slug": a.Slug, "name": a.Name, "count": a.Count, "threshold": a.Threshold,
			"window_sec": a.WindowSec, "doctor_agent": a.DoctorAgent, "self_repair_enabled": a.SelfRepairEnabled,
			"escalate_to": a.EscalateTo, "last_rollback_ms": a.LastRollbackMS, "task_type": a.TaskType,
			"current_chain": a.CurrentChain, "previous_chain": a.PreviousChain, "last_reason": a.LastReason,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"dead_agents":                     agents,
		"dead_count":                      len(agents),
		"degraded_agents":                 degraded,
		"degraded_count":                  len(degraded),
		"misconfigured_agents":            misconfigured,
		"misconfigured_count":             len(misconfigured),
		"retry_pressure_agents":           retryPressure,
		"retry_pressure_count":            len(retryPressure),
		"routing_pressure_agents":         routing,
		"routing_pressure_count":          len(routing),
		"routing_forced_probation_agents": forced,
		"routing_forced_probation_count":  len(forced),
		"routing_forced_failed_agents":    forcedFailed,
		"routing_forced_failed_count":     len(forcedFailed),
		"routing_forced_exhausted_agents": forcedExhausted,
		"routing_forced_exhausted_count":  len(forcedExhausted),
		"routing_unstable_agents":         unstable,
		"routing_unstable_count":          len(unstable),
		"stale_artifacts":                 rep.StaleArtifacts,
		"stale_bytes":                     rep.StaleBytes,
		"idle_days":                       idleDays,
		"stale_days":                      staleDays,
	}})
}

// intArg reads an integer-ish arg (JSON numbers arrive as float64), applying a
// floor of 1 and a default when absent or non-positive.
func intArg(raw any, def int) int {
	n := def
	switch v := raw.(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case int64:
		n = int(v)
	}
	if n < 1 {
		n = def
	}
	return n
}
