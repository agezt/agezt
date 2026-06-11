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
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"dead_agents":     agents,
		"dead_count":      len(agents),
		"stale_artifacts": rep.StaleArtifacts,
		"stale_bytes":     rep.StaleBytes,
		"idle_days":       idleDays,
		"stale_days":      staleDays,
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
