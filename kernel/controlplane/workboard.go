// SPDX-License-Identifier: MIT

// Workboard view helpers + List: workboardTaskView, workboardDependencyStateViews, workboardDependencySummary, handleWorkboardList.
// Code extracted from workboard.go during the Day-48 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/workboard"
)



func workboardTaskView(t workboard.Task) map[string]any {
	b, _ := json.Marshal(t)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["comment_count"] = len(t.Comments)
	m["link_count"] = len(t.Links)
	m["attempt_count"] = len(t.Attempts)
	m["failed_attempt_count"] = workboard.FailedAttemptCount(t)
	if len(t.Criteria) > 0 {
		met := 0
		for _, c := range t.Criteria {
			if c.Met {
				met++
			}
		}
		m["criteria_count"] = len(t.Criteria)
		m["criteria_met"] = met
		m["gated"] = true
		m["proven"] = t.Proof != nil && t.Proof.Satisfied()
	}
	if decision := workboard.RetryDecisionFor(t, ""); decision.MaxAttempts > 0 {
		m["max_attempts"] = decision.MaxAttempts
		if decision.NextAttempt > 0 {
			m["next_attempt"] = decision.NextAttempt
		}
	}
	return m
}

func workboardDependencyStateViews(states []workboard.DependencyState) []map[string]any {
	out := make([]map[string]any, 0, len(states))
	for _, st := range states {
		row := map[string]any{
			"id":     st.ID,
			"status": string(st.Status),
		}
		if st.Title != "" {
			row["title"] = st.Title
		}
		if st.Missing {
			row["missing"] = true
		}
		if st.CreatedMS > 0 {
			row["created_ms"] = st.CreatedMS
		}
		out = append(out, row)
	}
	return out
}

func workboardDependencySummary(states []workboard.DependencyState) string {
	parts := make([]string, 0, len(states))
	for _, st := range states {
		status := string(st.Status)
		if st.Missing {
			status = "missing"
		}
		if st.Title != "" {
			parts = append(parts, fmt.Sprintf("%s(%s:%s)", st.ID, st.Title, status))
		} else {
			parts = append(parts, fmt.Sprintf("%s(%s)", st.ID, status))
		}
	}
	return strings.Join(parts, ", ")
}

func (s *Server) handleWorkboardList(conn net.Conn, req Request) {
	var filter workboard.Filter
	if raw := stringArg(req.Args, "status"); raw != "" {
		st, err := workboard.ParseStatus(raw)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		filter.Status = st
	}
	filter.Tenant = stringArg(req.Args, "tenant")
	filter.Assignee = stringArg(req.Args, "assignee")
	if v, _, err := argBool(req.Args, "include_archived"); err != nil {
		s.fail(conn, req, err)
		return
	} else {
		filter.IncludeArchived = v
	}
	filter.Limit = intArg(req.Args["limit"], 100)
	tasks := s.k.Workboard().List(filter)
	out := make([]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, workboardTaskView(t))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tasks": out, "count": len(out)}})
}
