// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Workboard view helpers + List: workboardTaskView,
//             workboardDependencyStateViews, workboardDependencySummary,
//             handleWorkboardList. Code extracted from workboard.go during the
//             Day-48 god-file split. Public API unchanged.

import (
	"context"
	"encoding/json"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"net"

	"github.com/agezt/agezt/kernel/workboard"
)

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
