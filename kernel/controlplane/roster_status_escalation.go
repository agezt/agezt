// SPDX-License-Identifier: MIT

// Escalation/routing helpers: uniqueStrings + agentEscalationLoadViews + agentRoutingMatchesProfile.
// Code extracted from roster_status.go during the Day-39 god-file split. Public API unchanged.
package controlplane


import (
	"strings"

	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/roster"
)


func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	var prev string
	for _, s := range in {
		if s == "" || s == prev {
			continue
		}
		out = append(out, s)
		prev = s
	}
	return out
}

func (s *Server) agentEscalationLoadViews(profiles []roster.Profile) map[string]agentEscalationLoad {
	if len(profiles) == 0 {
		return nil
	}
	st, err := s.boardReader()
	if err != nil || st == nil {
		return nil
	}
	known := make(map[string]bool, len(profiles))
	out := make(map[string]agentEscalationLoad, len(profiles))
	for _, p := range profiles {
		known[strings.ToLower(strings.TrimSpace(p.Slug))] = true
	}
	for _, msg := range st.OpenHelp(boardReadMaxLimit) {
		to := strings.ToLower(strings.TrimSpace(msg.To))
		if to == "" {
			continue
		}
		if to == board.Everyone {
			for slug := range known {
				if strings.EqualFold(strings.TrimSpace(msg.From), slug) {
					continue
				}
				row := out[slug]
				if boardMessageAckedBy(msg, slug) {
					row.Acked++
				} else {
					row.Open++
				}
				out[slug] = row
			}
			continue
		}
		if !known[to] {
			continue
		}
		row := out[to]
		if boardMessageAckedBy(msg, to) {
			row.Acked++
		} else {
			row.Open++
		}
		out[to] = row
	}
	return out
}

func agentRoutingMatchesProfile(p roster.Profile, taskType, failedModel, nextModel string) bool {
	taskType = strings.TrimSpace(taskType)
	failedModel = strings.TrimSpace(failedModel)
	nextModel = strings.TrimSpace(nextModel)
	if pt := strings.TrimSpace(p.TaskType); pt != "" && strings.EqualFold(pt, taskType) {
		return true
	}
	for _, model := range agentModelChain(strings.TrimSpace(p.Model), p.Fallbacks) {
		if strings.EqualFold(model, failedModel) || strings.EqualFold(model, nextModel) {
			return true
		}
	}
	return false
}