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
	model, _ := req.Args["model"].(string)

	var e cadence.Entry
	var err error
	// Daily (wall-clock) when at_minutes is present; interval otherwise.
	if atAny, ok := req.Args["at_minutes"]; ok {
		at, _ := atAny.(float64)
		e, err = s.k.Schedules().AddDaily(intent, int(at), model, cadence.SourceOperator, time.Now())
	} else {
		sec, _ := req.Args["interval_sec"].(float64) // JSON numbers decode to float64
		if sec < 1 {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.interval_sec must be >= 1 (or pass at_minutes)"})
			return
		}
		e, err = s.k.Schedules().Add(intent, time.Duration(sec)*time.Second, model, cadence.SourceOperator, time.Now())
	}
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id": e.ID, "intent": e.Intent, "mode": e.Mode, "interval_sec": e.IntervalSec,
			"at_minutes": e.AtMinutes, "model": e.Model, "next_run_unix": e.NextRunUnix,
		},
	})
}

func (s *Server) handleScheduleEnable(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	enabled, _ := req.Args["enabled"].(bool)
	ok, err := s.k.Schedules().SetEnabled(id, enabled)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": ok, "enabled": enabled}})
}

func (s *Server) handleScheduleList(conn net.Conn, req Request) {
	entries := s.k.Schedules().List()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id": e.ID, "intent": e.Intent, "mode": e.Mode, "interval_sec": e.IntervalSec,
			"at_minutes": e.AtMinutes, "cadence": e.Cadence(),
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
