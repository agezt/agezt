// SPDX-License-Identifier: MIT

package controlplane

// Chronos standing-order CRUD handlers — the management path behind `agt
// standing`. Lifecycle changes go through the kernel so every create/pause/
// resume/remove is journaled (standing.*) and auditable via `agt why`.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/standing"
)

// standingView is the stable wire shape for one order.
func standingView(o standing.Order) map[string]any {
	b, _ := json.Marshal(o)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func (s *Server) handleStandingList(conn net.Conn, req Request) {
	orders := s.k.Standing().List()
	out := make([]any, 0, len(orders))
	enabled := 0
	for _, o := range orders {
		out = append(out, standingView(o))
		if o.Enabled {
			enabled++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"orders": out, "count": len(out), "enabled_count": enabled},
	})
}

func (s *Server) handleStandingAdd(conn net.Conn, req Request) {
	raw, ok := req.Args["order"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.order required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.order: " + err.Error()})
		return
	}
	var o standing.Order
	if err := json.Unmarshal(b, &o); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.order: " + err.Error()})
		return
	}
	saved, err := s.k.AddStanding(o)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"order": standingView(saved)}})
}

func (s *Server) handleStandingSetEnabled(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	enabled, _ := req.Args["enabled"].(bool)
	o, err := s.k.SetStandingEnabled(id, enabled)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"order": standingView(o)}})
}

func (s *Server) handleStandingRemove(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	removed, err := s.k.RemoveStanding(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed, "id": id}})
}
