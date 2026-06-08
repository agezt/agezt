// SPDX-License-Identifier: MIT

package controlplane

// Tool inventory handler — exposes the in-process tools the kernel
// will advertise to the model. Operator-facing: `agt tool list`
// uses this to confirm a plugin's tool actually registered, which
// is the first question to answer when a model isn't calling a
// tool the operator expected it to call.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/edict"
)

// catalogProbe holds a representative input per input-branching tool so the
// catalog can report the tool's PRIMARY governed capability — the higher-risk
// axis an operator most wants to see the policy for. Tools that don't branch on
// input map to one capability regardless, so they're absent here (nil input).
var catalogProbe = map[string]json.RawMessage{
	"file":          json.RawMessage(`{"op":"write"}`),
	"http":          json.RawMessage(`{"method":"POST"}`),
	"homeassistant": json.RawMessage(`{"operation":"call_service"}`),
}

// handleToolList serves CmdToolList. Returns a deterministic,
// name-sorted list so two consecutive calls produce identical
// output (Go map iteration is randomized, so we sort here rather
// than make the client do it). Each row carries the tool's governing
// Edict capability (its primary axis) so the operator can see the full
// agent capability surface and cross-reference it with the policy levels.
func (s *Server) handleToolList(conn net.Conn, req Request) {
	tools := s.k.Tools()
	rows := make([]map[string]any, 0, len(tools))
	for name, t := range tools {
		def := t.Definition()
		cap := edict.CapabilityForToolCall(name, catalogProbe[name])
		rows = append(rows, map[string]any{
			"name":        def.Name,
			"description": def.Description,
			"capability":  string(cap),
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
