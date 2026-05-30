// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
)

// Scheduled-intents handlers (autonomy) — the control-plane surface behind
// `agt schedule`. Writes go to the kernel's persistent cadence.Store; the
// cadence resident fires due entries through the governed loop. Operators manage
// only operator-sourced entries here; env-seeded ones come from AGEZT_SCHEDULE.

func (s *Server) handleScheduleAdd(conn net.Conn, req Request) {
	intent, _ := req.Args["intent"].(string)
	if intent == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.intent required"})
		return
	}
	sec, _ := req.Args["interval_sec"].(float64) // JSON numbers decode to float64
	if sec < 1 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.interval_sec must be >= 1"})
		return
	}
	model, _ := req.Args["model"].(string)

	e, err := s.k.Schedules().Add(intent, time.Duration(sec)*time.Second, model, cadence.SourceOperator, time.Now())
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id": e.ID, "intent": e.Intent, "interval_sec": e.IntervalSec,
			"model": e.Model, "next_run_unix": e.NextRunUnix,
		},
	})
}

func (s *Server) handleScheduleList(conn net.Conn, req Request) {
	entries := s.k.Schedules().List()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id": e.ID, "intent": e.Intent, "interval_sec": e.IntervalSec,
			"model": e.Model, "source": e.Source, "enabled": e.Enabled,
			"created_unix": e.CreatedUnix, "last_run_unix": e.LastRunUnix,
			"next_run_unix": e.NextRunUnix,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"schedules": out, "count": len(out)},
	})
}

func (s *Server) handleScheduleRemove(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	removed, err := s.k.Schedules().Remove(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed}})
}

func (s *Server) handleScheduleRun(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	triggered, err := s.k.Schedules().RunNow(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"triggered": triggered}})
}
