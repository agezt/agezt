// SPDX-License-Identifier: MIT
package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/roster"
	"strconv"
	"strings"
)

func agentModelChain(primary string, fallbacks []string) []string {
	chain := []string{primary}
	for _, m := range fallbacks {
		if m = strings.TrimSpace(m); m != "" && m != primary {
			chain = append(chain, m)
		}
	}
	return chain
}

func profileView(p roster.Profile) map[string]any { return approster.ProfileView(p) }
func (s *Server) rosterListService() *approster.ListService {
	s.rosterListOnce.Do(func() {
		s.rosterList = approster.NewList(func() []roster.Profile { return s.k.Roster().List() }, s.agentStatusViews, nil)
	})
	return s.rosterList
}
func (s *Server) invalidateAgentListCache() { s.rosterListService().Invalidate() }

// parseSeqCursor extracts the opaque "<seq>" cursor used by the journal-sorted
// endpoints (agents/activity, agents/repair_status). Returns (0, false) for a
// missing, empty, or unparseable cursor — callers treat that as "no cursor,
// return first page."
func parseSeqCursor(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}
