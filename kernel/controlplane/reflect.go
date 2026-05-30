// SPDX-License-Identifier: MIT

package controlplane

// Reflection inspection/trigger handlers — the path behind `agt reflect`.
// `run` triggers one reflection pass (folds the journal, applies world-model
// decay, journals the report); `show` reads the latest report back. Both go
// through the kernel's reflect.Engine so the decay it applies is journaled
// under the pass's correlation and explainable via `agt why`.

import (
	"context"
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/reflect"
	"github.com/agezt/agezt/kernel/ulid"
)

func (s *Server) handleReflectRun(conn net.Conn, req Request) {
	corr := "reflect-" + ulid.New()
	rep, err := s.k.Reflect().Reflect(context.Background(), corr)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := reportView(rep)
	result["correlation_id"] = corr
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func (s *Server) handleReflectShow(conn net.Conn, req Request) {
	rep, ok := s.k.Reflect().Latest()
	result := map[string]any{"found": ok}
	if ok {
		result["report"] = reportView(rep)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// reportView renders a reflect.Report as a stable map for the wire by reusing
// its JSON tags (avoids hand-copying every field).
func reportView(rep reflect.Report) map[string]any {
	b, _ := json.Marshal(rep)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}
