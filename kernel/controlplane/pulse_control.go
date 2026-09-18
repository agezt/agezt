// SPDX-License-Identifier: MIT

package controlplane

// Pulse control handlers — status / pause / resume for the resident
// proactive engine (SPEC-03). The control plane stays decoupled from
// kernel/pulse: it talks to a PulseController interface that the daemon
// injects via SetPulse. When Pulse is disabled (no engine wired), the
// handlers answer "disabled" rather than erroring, so `agt pulse status`
// is always safe to call.

import (
	"net"
	"strconv"
	"time"
)


// persistPulseSetting writes a live pulse setting to the config store (M760) so it
// survives restart: buildPulse reads these env vars at startup, and the config store
// is overlaid onto the environment first, so a persisted value becomes the new default.
// Best-effort — a store failure never fails the live change, which already took effect.
func (s *Server) SetPulse(p PulseController) { s.pulse = p }

func (s *Server) handlePulseStatus(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"enabled": false}})
		return
	}
	res := s.pulse.StatusMap()
	res["enabled"] = true
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handlePulsePause(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	s.pulse.Pause()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"paused": true}})
}

func (s *Server) handlePulseResume(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	s.pulse.Resume()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"paused": false}})
}

// handlePulseBeat triggers one on-demand heartbeat (M756) — "think now". Returns as
// soon as the beat is queued; the observations/initiatives it produces surface in the
// autonomy feed asynchronously, like a scheduled tick. Fires even when paused.
func (s *Server) handlePulseBeat(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	s.pulse.Beat()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"triggered": true}})
}

// handlePulseAsks lists the actionable observations awaiting an operator verdict
// under initiative=ask (M1001) — what the Jarvis presence pillar renders so the ask
// path isn't a silent dead-end.
func (s *Server) handlePulseAsks(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"asks": []any{}}})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"asks": s.pulse.PendingAsks()}})
}

// handlePulseAskResolve settles one pending ask (M1001). args.issue_key picks it;
// args.approve (bool, or "true"/"false" string from the webui) decides — approval
// re-emits the signal onto pulse.initiative.act so the responder can act on it.
func (s *Server) handlePulseAskResolve(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	key, err := requiredArgString(req.Args, "issue_key")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	approve := false
	switch v := req.Args["approve"].(type) {
	case bool:
		approve = v
	case string:
		approve = v == "true" || v == "1"
	}
	found, acted := s.pulse.ResolveAsk(key, approve)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no pending ask with that issue_key (already resolved?)"})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"resolved": true, "approved": approve, "acted": acted}})
}

// handlePulseCadence changes the heartbeat interval live (M757). seconds may arrive
// as a number (CLI/JSON) or a string (webui query arg). Returns the applied cadence
// (clamped by the engine to a sane range).
func (s *Server) handlePulseCadence(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	var secs float64
	switch v := req.Args["seconds"].(type) {
	case float64:
		secs = v
	case string:
		secs, _ = strconv.ParseFloat(v, 64)
	}
	if secs <= 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.seconds must be > 0"})
		return
	}
	applied := s.pulse.SetCadence(time.Duration(secs * float64(time.Second)))
	s.persistPulseSetting("AGEZT_PULSE_CADENCE", applied.String())
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"cadence_ms": applied.Milliseconds()}})
}
func registerPulseControlCommands() {
	register(
		commandSpec{Cmd: CmdPulseStatus, Handler: func(dc *DispatchCtx) { dc.S.handlePulseStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseAsks, Handler: func(dc *DispatchCtx) { dc.S.handlePulseAsks(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseAskResolve, Handler: func(dc *DispatchCtx) { dc.S.handlePulseAskResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulsePause, Handler: func(dc *DispatchCtx) { dc.S.handlePulsePause(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseResume, Handler: func(dc *DispatchCtx) { dc.S.handlePulseResume(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseBeat, Handler: func(dc *DispatchCtx) { dc.S.handlePulseBeat(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseCadence, Handler: func(dc *DispatchCtx) { dc.S.handlePulseCadence(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseDial, Handler: func(dc *DispatchCtx) { dc.S.handlePulseDial(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseFlush, Handler: func(dc *DispatchCtx) { dc.S.handlePulseFlush(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseWatch, Handler: func(dc *DispatchCtx) { dc.S.handlePulseWatch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseProbe, Handler: func(dc *DispatchCtx) { dc.S.handlePulseProbe(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseUnwatch, Handler: func(dc *DispatchCtx) { dc.S.handlePulseUnwatch(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPulseQuiet, Handler: func(dc *DispatchCtx) { dc.S.handlePulseQuiet(dc.Conn, dc.Req) }},
	)
}
