// SPDX-License-Identifier: MIT

package controlplane

// Tool inventory handler — exposes the in-process tools the kernel
// will advertise to the model. Operator-facing: `agt tool list`
// uses this to confirm a plugin's tool actually registered, which
// is the first question to answer when a model isn't calling a
// tool the operator expected it to call.

import (
	"net"
	"sort"
)

// handleToolList serves CmdToolList. Returns a deterministic,
// name-sorted list so two consecutive calls produce identical
// output (Go map iteration is randomized, so we sort here rather
// than make the client do it).
func (s *Server) handleToolList(conn net.Conn, req Request) {
	tools := s.k.Tools()
	rows := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		def := t.Definition()
		rows = append(rows, map[string]any{
			"name":        def.Name,
			"description": def.Description,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		ni, _ := rows[i]["name"].(string)
		nj, _ := rows[j]["name"].(string)
		return ni < nj
	})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"tools": rows,
			"count": len(rows),
		},
	})
}
