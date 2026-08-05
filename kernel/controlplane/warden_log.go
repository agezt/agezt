// SPDX-License-Identifier: MIT

package controlplane

// Warden execution log (M96) — a read-only timeline of the journal's
// warden.executed / warden.profile_downgraded / warden.limit_exceeded events.
// The Warden is the OS-level sandbox that runs shell/process tool calls under a
// resource+isolation profile; it journals each execution (effective profile,
// exit code, duration), each profile downgrade (requested isolation unavailable
// on this host), and each limit breach (output/time cap hit). `agt edict log`
// audits POLICY gating and `agt approvals log` the HUMAN gating; this audits the
// SANDBOX — the third pillar of the security model, previously unsurfaced.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

// handleWardenStats aggregates sandboxed executions (M97) — total execs,
// downgraded count + rate, timed-out count, a by-effective-profile breakdown,
// and the count of limit breaches. Answers "is my sandbox actually isolating, or
// silently degrading?". since_ms windows by event time. Tenant-routed.
func (s *Server) handleWardenStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	var sinceMS int64
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var total, downgraded, timedOut, limitBreaches int
	byProfile := map[string]int{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		switch e.Kind {
		case event.KindWardenExecuted:
			var p struct {
				ProfileEffective string `json:"profile_effective"`
				Downgraded       bool   `json:"downgraded"`
				TimedOut         bool   `json:"timed_out"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			total++
			prof := p.ProfileEffective
			if prof == "" {
				prof = "unknown"
			}
			byProfile[prof]++
			if p.Downgraded {
				downgraded++
			}
			if p.TimedOut {
				timedOut++
			}
		case event.KindWardenLimitExceeded:
			limitBreaches++
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	downgradeRate := 0.0
	if total > 0 {
		downgradeRate = float64(downgraded) / float64(total)
	}
	byProfileOut := make(map[string]any, len(byProfile))
	for n, c := range byProfile {
		byProfileOut[n] = c
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"executions":     total,
			"downgraded":     downgraded,
			"downgrade_rate": downgradeRate,
			"timed_out":      timedOut,
			"limit_breaches": limitBreaches,
			"by_profile":     byProfileOut,
			"window_ms":      sinceMS,
		},
	})
}

func (s *Server) handleWardenLog(conn net.Conn, req Request) {
	// --downgrades keeps only the noteworthy events (downgrades + limit breaches).
	issuesOnly, _ := req.Args["issues"].(bool)
	s.projectJournal(conn, req, "executions", func(e *event.Event) (map[string]any, bool) {
		switch e.Kind {
		case event.KindWardenExecuted:
			if issuesOnly {
				return nil, false
			}
			var p struct {
				ProfileEffective string `json:"profile_effective"`
				Argv0            string `json:"argv0"`
				ExitCode         int    `json:"exit_code"`
				DurationMS       int64  `json:"duration_ms"`
				Downgraded       bool   `json:"downgraded"`
				TimedOut         bool   `json:"timed_out"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			return map[string]any{
				"kind": "exec", "profile": p.ProfileEffective, "argv0": p.Argv0,
				"exit_code": p.ExitCode, "duration_ms": p.DurationMS,
				"downgraded": p.Downgraded, "timed_out": p.TimedOut,
			}, true
		case event.KindWardenProfileDowngraded:
			var p struct {
				Requested string `json:"requested"`
				Effective string `json:"effective"`
				Reason    string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			return map[string]any{
				"kind": "downgrade", "profile": p.Effective,
				"requested": p.Requested, "reason": p.Reason,
			}, true
		case event.KindWardenLimitExceeded:
			var p struct {
				Limit string `json:"limit"`
				Argv0 string `json:"argv0"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			return map[string]any{
				"kind": "limit", "profile": "", "argv0": p.Argv0, "reason": p.Limit,
			}, true
		default:
			return nil, false
		}
	})
}
