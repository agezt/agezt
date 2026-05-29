// SPDX-License-Identifier: MIT

package controlplane

// External plugin inventory handler. Daemon-supplied manifest
// (see runtime.PluginInfo); kernel itself never spawns plugins.
// Read-only — surfaces what was wired at startup so operators
// can verify "did my plugin actually load?" without grepping
// daemon stderr.

import (
	"net"
	"sort"
)

func (s *Server) handlePluginList(conn net.Conn, req Request) {
	plugins := s.k.Plugins()
	rows := make([]map[string]any, 0, len(plugins))
	for _, p := range plugins {
		// Copy Args / AllowedTools to []any so the JSON encoder
		// emits arrays (Go []string serializes fine, but []any keeps
		// the shape uniform with the rest of the control-plane
		// responses that use map[string]any throughout).
		args := make([]any, len(p.Args))
		for i, a := range p.Args {
			args[i] = a
		}
		var allowed []any
		if p.AllowedTools != nil {
			allowed = make([]any, len(p.AllowedTools))
			for i, t := range p.AllowedTools {
				allowed[i] = t
			}
		}
		rows = append(rows, map[string]any{
			"prefix":        p.Prefix,
			"path":          p.Path,
			"args":          args,
			"tool_count":    p.ToolCount,
			"hash_pinned":   p.HashPinned,
			"allowed_tools": allowed, // nil → JSON null = "unrestricted"
		})
	}
	// Sort by prefix for deterministic output. Operators reading
	// `agt plugin list` expect stable order across calls.
	sort.Slice(rows, func(i, j int) bool {
		pi, _ := rows[i]["prefix"].(string)
		pj, _ := rows[j]["prefix"].(string)
		return pi < pj
	})

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"plugins": rows,
			"count":   len(rows),
		},
	})
}
