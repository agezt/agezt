// SPDX-License-Identifier: MIT

package controlplane

// Rate-limit observability (M106). The governor enforces a per-minute call cap
// (AGEZT_RATE_PER_MIN for the primary, AGEZT_TENANT_RATE_PER_MIN per tenant) and
// journals a rate.limited event whenever it refuses a call. Those events were
// only reachable via `agt journal grep --kind rate.limited`; this folds them
// into a first-class surface so an operator can see, per tenant, whether callers
// are being throttled — silent throttling is an SRE blind spot. Mirrors the
// edict/warden/approvals log+stats pattern.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleRateLimitLog(conn net.Conn, req Request) {
	s.projectJournal(conn, req, "throttles", func(e *event.Event) (map[string]any, bool) {
		if e.Kind != event.KindRateLimited {
			return nil, false
		}
		var p struct {
			Used        int `json:"used"`
			LimitPerMin int `json:"limit_per_min"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		return map[string]any{"used": p.Used, "limit_per_min": p.LimitPerMin}, true
	})
}

func (s *Server) handleRateLimitStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	sinceMS := int64Arg(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	total := 0
	limitN := 0
	worstUsed := 0
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindRateLimited {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			Used        int `json:"used"`
			LimitPerMin int `json:"limit_per_min"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		total++
		if p.LimitPerMin > 0 {
			limitN = p.LimitPerMin
		}
		if p.Used > worstUsed {
			worstUsed = p.Used
		}
		return nil
	}); err != nil {
		s.fail(conn, req, err)
		return
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"throttled":     total,
			"limit_per_min": limitN,
			"worst_used":    worstUsed,
			"window_ms":     sinceMS,
		},
	})
}
