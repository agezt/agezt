// SPDX-License-Identifier: MIT

// Control-plane schedule-stats handler.
// Code extracted from schedule_fires.go during the Day-86 god-file split.
// Public API unchanged.
package controlplane


import (
	"net"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleScheduleStats(conn net.Conn, req Request) {
	idFilter, _, err := argString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	sinceMS := int64Arg(req.Args["since_ms"])
	var cutoff int64
	if sinceMS > 0 {
		cutoff = time.Now().UnixMilli() - sinceMS
	}

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	runs, err := s.collectRuns(k)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	var total, completed, failed, running, abandoned int
	var spent int64
	failedByReason := map[string]int{}
	scheduleSet := map[string]struct{}{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindScheduleFired {
			return nil
		}
		id := extractScheduleFired(e.Payload).ScheduleID
		if idFilter != "" && id != idFilter {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		total++
		if id != "" {
			scheduleSet[id] = struct{}{}
		}
		// Join with the firing's run outcome.
		if r, ok := runs[e.CorrelationID]; ok {
			switch {
			case r.Completed:
				completed++
			case r.Failed:
				failed++
				reason := r.FailReason
				if reason == "" {
					reason = "unknown"
				}
				failedByReason[reason]++
			case r.Abandoned:
				abandoned++
			default:
				running++
			}
			spent += r.SpentMicrocents
		} else {
			running++ // fired but no run events yet (or trimmed)
		}
		return nil
	}); err != nil {
		s.fail(conn, req, err)
		return
	}

	terminal := completed + failed + abandoned
	successRate := 0.0
	if terminal > 0 {
		successRate = float64(completed) / float64(terminal)
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":            total,
			"completed":        completed,
			"failed":           failed,
			"running":          running,
			"abandoned":        abandoned,
			"terminal":         terminal,
			"success_rate":     successRate,
			"spent_microcents": spent,
			"schedules":        len(scheduleSet),
			"failed_by_reason": failedByReason,
			"window_ms":        sinceMS,
		},
	})
}
