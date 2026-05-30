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
	// One-shot when once_at_unix is present; daily when at_minutes is present;
	// interval otherwise.
	if onceAny, ok := req.Args["once_at_unix"]; ok {
		at, _ := onceAny.(float64)
		e, err = s.k.Schedules().AddOnce(intent, time.Unix(int64(at), 0), model, cadence.SourceOperator, time.Now())
	} else if startAny, ok := req.Args["window_start"]; ok {
		start, _ := startAny.(float64)
		end, _ := req.Args["window_end"].(float64)
		sec, _ := req.Args["interval_sec"].(float64)
		days, _ := req.Args["days"].(float64)
		tz, _ := req.Args["tz"].(string)
		e, err = s.k.Schedules().AddWindow(intent, time.Duration(sec)*time.Second, int(start), int(end), int(days), tz, model, cadence.SourceOperator, time.Now())
	} else if atAny, ok := req.Args["at_minutes"]; ok {
		at, _ := atAny.(float64)
		days, _ := req.Args["days"].(float64) // weekday bitmask; 0 = every day
		tz, _ := req.Args["tz"].(string)
		e, err = s.k.Schedules().AddDaily(intent, int(at), int(days), tz, model, cadence.SourceOperator, time.Now())
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
			"at_minutes": e.AtMinutes, "end_minutes": e.EndMinutes, "days": e.Days,
			"model": e.Model, "next_run_unix": e.NextRunUnix,
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

func (s *Server) handleScheduleEdit(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	store := s.k.Schedules()
	if _, ok := store.Get(id); !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": false}})
		return
	}
	now := time.Now()

	// Field edits (any subset). A failure on intent (empty) is reported.
	if v, ok := req.Args["intent"]; ok {
		intent, _ := v.(string)
		if _, err := store.SetIntent(id, intent); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}
	if v, ok := req.Args["model"]; ok {
		model, _ := v.(string)
		_, _ = store.SetModel(id, model)
	}

	// At most one cadence change: once | window | daily | interval.
	var err error
	tz, _ := req.Args["tz"].(string)
	if v, ok := req.Args["once_at_unix"]; ok {
		at, _ := v.(float64)
		_, err = store.Reschedule(id, cadence.ModeOnce, 0, 0, 0, 0, "", time.Unix(int64(at), 0), now)
	} else if v, ok := req.Args["window_start"]; ok {
		start, _ := v.(float64)
		end, _ := req.Args["window_end"].(float64)
		sec, _ := req.Args["interval_sec"].(float64)
		days, _ := req.Args["days"].(float64)
		_, err = store.Reschedule(id, cadence.ModeWindow, time.Duration(sec)*time.Second, int(start), int(end), int(days), tz, time.Time{}, now)
	} else if v, ok := req.Args["at_minutes"]; ok {
		at, _ := v.(float64)
		days, _ := req.Args["days"].(float64)
		_, err = store.Reschedule(id, cadence.ModeDaily, 0, int(at), 0, int(days), tz, time.Time{}, now)
	} else if v, ok := req.Args["interval_sec"]; ok {
		sec, _ := v.(float64)
		_, err = store.Reschedule(id, cadence.ModeInterval, time.Duration(sec)*time.Second, 0, 0, 0, "", time.Time{}, now)
	}
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	e, _ := store.Get(id)
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"updated": true, "id": e.ID, "mode": e.Mode, "cadence": e.Cadence(),
			"intent": e.Intent, "next_run_unix": e.NextRunUnix,
		},
	})
}

func (s *Server) handleScheduleList(conn net.Conn, req Request) {
	entries := s.k.Schedules().List()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id": e.ID, "intent": e.Intent, "mode": e.Mode, "interval_sec": e.IntervalSec,
			"at_minutes": e.AtMinutes, "end_minutes": e.EndMinutes, "days": e.Days, "tz": e.TZ, "cadence": e.Cadence(),
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
