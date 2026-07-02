// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/seat"
)

func seatView(st seat.Seat) map[string]any {
	b, _ := json.Marshal(st)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// handleSeatList returns the execution seats a workboard task can be dispatched
// under: seeded built-ins plus operator-defined custom seats.
func (s *Server) handleSeatList(conn net.Conn, req Request) {
	seats := s.k.Seats().List()
	out := make([]any, 0, len(seats))
	for _, st := range seats {
		out = append(out, seatView(st))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"seats": out, "count": len(out)}})
}

// handleSeatCreate adds a custom seat.
func (s *Server) handleSeatCreate(conn net.Conn, req Request) {
	spec := seat.Seat{
		ID:               stringArg(req.Args, "id"),
		Name:             stringArg(req.Args, "name"),
		Description:      stringArg(req.Args, "description"),
		ExecutionProfile: stringArg(req.Args, "execution_profile"),
		ModelChain:       workboardStringSliceArg(req.Args["model_chain"]),
		Tools:            workboardStringSliceArg(req.Args["tools"]),
	}
	if _, ok := req.Args["restrict_tools"].(bool); ok {
		spec.RestrictTools, _ = req.Args["restrict_tools"].(bool)
	}
	made, err := s.k.Seats().Create(spec)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"seat": seatView(made)}})
}

// handleSeatDelete removes a custom seat (built-ins are refused).
func (s *Server) handleSeatDelete(conn net.Conn, req Request) {
	id := stringArg(req.Args, "id")
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "seat_delete requires id"})
		return
	}
	if err := s.k.Seats().Delete(id); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"deleted": id}})
}
