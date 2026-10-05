// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	appworld "github.com/agezt/agezt/kernel/app/world"
	"net"
)

func (s *Server) handleWorldLog(conn net.Conn, req Request) {
	kind, _, err := argString(req.Args, "kind")
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
	out, err := appworld.NewLog(k.Journal()).Log(context.Background(), appworld.LogInput{Limit: limit, CutoffMS: cutoff, Cursor: req.Args["cursor"], KindFilter: kind})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: body})
}
