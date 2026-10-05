// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Workboard view helpers + List: workboardTaskView,
//             workboardDependencyStateViews, workboardDependencySummary,
//             handleWorkboardList. Code extracted from workboard.go during the
//             Day-48 god-file split. Public API unchanged.

import (
	"context"
	"encoding/json"
	"fmt"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/workboard"
)

func workboardTaskView(task workboard.Task) map[string]any {
	raw, _ := json.Marshal(appworkboard.Project(task))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
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
	out, err := appworkboard.New(s.k.Workboard()).List(context.Background(), appworkboard.ListInput{Status: filter.Status, Tenant: filter.Tenant, Assignee: filter.Assignee, IncludeArchived: filter.IncludeArchived, Limit: filter.Limit})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	writeWorkboardReadResult(s, conn, req, out)
}

func writeWorkboardReadResult(s *Server, conn net.Conn, req Request, out any) {
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
