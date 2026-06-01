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

	// Delegation governance (M49): surface the active depth / fan-out / spend
	// ceilings (M46–M48) so an operator can see what's in effect — they were
	// silent until a delegation tripped one. 0 fan-out / spend = unbounded.
	dl := s.k.SubAgentLimits()

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"daemon":         brand.Version,
			"protocol":       brand.ProtocolVersion,
			"model":          s.k.Model(),
			"uptime_seconds": uptimeSecs,
			"halted":         s.k.IsHalted(),
			"active_runs":    s.k.ActiveRuns(),
			"tools":          len(s.k.Tools()),
			"memory_records": s.k.Memory().Count(),
			"world_entities": s.k.World().Count(),
			"active_skills":  s.k.Forge().Count(),
			"journal_head":   headSeq,
			"delegation": map[string]any{
				"enabled":              dl.Enabled,
				"max_depth":            dl.MaxDepth,
				"max_fanout":           dl.MaxFanout,
				"max_spend_microcents": dl.MaxSpendMicrocents,
			},
		},
	})
}
