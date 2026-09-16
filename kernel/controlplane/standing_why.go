// SPDX-License-Identifier: MIT
//
// kernel/controlplane standing-order audit handler (handleStandingWhy).
// Extracted from standing_handlers.go during Day 211 god-file refactor (#83).
// Public API unchanged.
package controlplane

import (
	"encoding/json"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleStandingWhy(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var events []any
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if !strings.HasPrefix(string(e.Kind), "standing.") {
			return nil
		}
		var p map[string]any
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		if p["id"] != id {
			return nil
		}
		events = append(events, map[string]any{
			"seq":            e.Seq,
			"id":             e.ID,
			"kind":           string(e.Kind),
			"correlation_id": e.CorrelationID,
			"ts_unix_ms":     e.TSUnixMS,
			"payload":        p,
		})
		return nil
	})
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"id": id, "events": events, "count": len(events)},
	})
}
