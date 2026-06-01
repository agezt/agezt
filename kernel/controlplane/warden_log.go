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
	"sort"

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
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	// --downgrades keeps only the noteworthy events (downgrades + limit breaches).
	issuesOnly, _ := req.Args["issues"].(bool)
	cutoff := sinceCutoff(req.Args["since_ms"]) // M65 helper

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type wardenEvent struct {
		ts, seq    int64
		kind       string // exec | downgrade | limit
		argv0      string
		profile    string
		exitCode   int
		durationMS int64
		downgraded bool
		timedOut   bool
		requested  string // downgrade: requested profile
		reason     string // downgrade reason / limit name
	}
	rows := make([]wardenEvent, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		switch e.Kind {
		case event.KindWardenExecuted:
			if issuesOnly {
				return nil
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
			rows = append(rows, wardenEvent{
				ts: e.TSUnixMS, seq: e.Seq, kind: "exec", argv0: p.Argv0,
				profile: p.ProfileEffective, exitCode: p.ExitCode, durationMS: p.DurationMS,
				downgraded: p.Downgraded, timedOut: p.TimedOut,
			})
		case event.KindWardenProfileDowngraded:
			var p struct {
				Requested string `json:"requested"`
				Effective string `json:"effective"`
				Reason    string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, wardenEvent{
				ts: e.TSUnixMS, seq: e.Seq, kind: "downgrade",
				requested: p.Requested, profile: p.Effective, reason: p.Reason,
			})
		case event.KindWardenLimitExceeded:
			var p struct {
				Limit string `json:"limit"`
				Argv0 string `json:"argv0"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, wardenEvent{
				ts: e.TSUnixMS, seq: e.Seq, kind: "limit", argv0: p.Argv0, reason: p.Limit,
			})
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts > rows[j].ts
		}
		return rows[i].seq > rows[j].seq
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		m := map[string]any{"ts_unix_ms": r.ts, "kind": r.kind, "profile": r.profile}
		switch r.kind {
		case "exec":
			m["argv0"] = r.argv0
			m["exit_code"] = r.exitCode
			m["duration_ms"] = r.durationMS
			m["downgraded"] = r.downgraded
			m["timed_out"] = r.timedOut
		case "downgrade":
			m["requested"] = r.requested
			m["reason"] = r.reason
		case "limit":
			m["argv0"] = r.argv0
			m["reason"] = r.reason
		}
		out = append(out, m)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"executions": out, "count": len(out)},
	})
}
