// SPDX-License-Identifier: MIT

package controlplane

// Daemon health overview. Operators (and CI smoke tests) want a
// single round-trip that answers "is the daemon up, what version
// is it, and what's it doing right now?" — instead of dialing
// CmdVersion + CmdBudget + CmdToolList separately and reconciling
// the results client-side.
//
// Read-only. No mutation, no side effects on the kernel.

import (
	"net"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

func (s *Server) handleStatus(conn net.Conn, req Request) {
	headSeq, _ := s.k.Journal().Head() // (seq, hash); hash unused here
	// Journal.Head returns -1 on an empty journal (nextSeq-1), which
	// is a leaky implementation detail for an operator dashboard.
	// Clamp to 0 so `agt status` renders "head seq=0" on a fresh
	// install rather than the confusing "-1".
	if headSeq < 0 {
		headSeq = 0
	}

	uptime := time.Since(s.k.StartTime())
	// Integer seconds — sub-second precision adds noise without
	// helping any operator workflow. Floor (not round) so a 0.9s
	// uptime renders as 0, matching "just started".
	uptimeSecs := int64(uptime / time.Second)

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"daemon":         brand.Version,
			"protocol":       brand.ProtocolVersion,
			"uptime_seconds": uptimeSecs,
			"halted":         s.k.IsHalted(),
			"active_runs":    s.k.ActiveRuns(),
			"tools":          len(s.k.Tools()),
			"memory_records": s.k.Memory().Count(),
			"world_entities": s.k.World().Count(),
			"journal_head":   headSeq,
		},
	})
}
