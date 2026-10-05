// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	"net"
)

func (s *Server) handleMemoryLog(conn net.Conn, req Request) {
	op, _, err := argString(req.Args, "op")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	cutoff := sinceCutoff(req.Args["since_ms"])
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out, err := appmemory.NewLog(k.Journal()).Log(context.Background(), appmemory.LogInput{Limit: limit, CutoffMS: cutoff, Cursor: req.Args["cursor"], OpFilter: op})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	body, err := jsonMap(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
