// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"strings"
	"time"
)

// Schedule handlers (autonomy) — the control-plane surface behind `agt
// schedule`. Writes go to the kernel's persistent cadence.Store; the cadence
// resident fires due entries as agent wakes, workflow runs, daemon tasks, or
// tool invocations. Operators manage only operator-sourced entries here;
// env-seeded ones come from AGEZT_SCHEDULE.

func (s *Server) handleScheduleEnable(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Accept enabled as a bool (CLI/JSON transport) or a "true"/"false"/"1"/"0"
	// string (webui query-arg transport, which carries every value as a string).
	enabled := false
	switch v := req.Args["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	}
	current, found := s.k.Schedules().Get(id)
	if enabled {
		if found {
			if err := s.validateScheduleRunnable(current); err != nil {
				s.fail(conn, req, err)
				return
			}
		}
	}
	ok, err := s.k.Schedules().SetEnabled(id, enabled)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	action := "paused"
	if enabled {
		action = "resumed"
	}
	if ok {
		publishOperatorAction(s.k, "schedule.enable", s.k.NewCorrelation(), map[string]any{
			"id":      id,
			"enabled": enabled,
			"action":  action,
			"target":  current.Target,
			"agent":   current.Agent,
			"cadence": current.Cadence(),
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": ok, "enabled": enabled, "id": id, "action": action}})
}

// handleScheduleTest previews a schedule's next fire times (M120) — a read-only
// dry-run. Uses the entry's own Forecast simulation so the result matches what
// the cadence engine will actually do.
func (s *Server) handleScheduleTest(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	count := 5
	if n, present, err := scheduleArgNumber(req.Args, "count"); err != nil {
		s.fail(conn, req, err)
		return
	} else if present {
		count = int(n)
	}
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}

	e, ok := s.k.Schedules().Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"found": false}})
		return
	}
	fires := e.Forecast(time.Now(), count)
	out := make([]map[string]any, 0, len(fires))
	for _, f := range fires {
		out = append(out, map[string]any{"unix": f})
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"found":     true,
			"id":        e.ID,
			"mode":      e.Mode,
			"cadence":   e.Cadence(),
			"enabled":   e.Enabled,
			"forecasts": out,
			"count":     len(out),
		},
	})
}

func (e errString) Error() string { return string(e) }

// registerScheduleCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerScheduleCommands() {
	register(
		commandSpec{Cmd: CmdScheduleAdd, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleList, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleSystemTasks, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleSystemTasks(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleRemove, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleRun, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleRun(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleEnable, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleEnable(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleEdit, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdScheduleTest, Handler: func(dc *DispatchCtx) { dc.S.handleScheduleTest(dc.Conn, dc.Req) }},
	)
}
