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
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/settings"
)

// persistPulseSetting writes a live pulse setting to the config store (M760) so it
// survives restart: buildPulse reads these env vars at startup, and the config store
// is overlaid onto the environment first, so a persisted value becomes the new default.
// Best-effort — a store failure never fails the live change, which already took effect.
func (s *Server) persistPulseSetting(name, value string) {
	store := settings.NewStore(s.baseDir)
	if err := store.Load(); err != nil {
		return
	}
	store.Set(name, value)
	_ = store.Save()
}

// PulseController is the slice of the Pulse engine the control plane needs.
// kernel/pulse.Engine satisfies it (StatusMap/Pause/Resume/Beat/SetCadence); the
// daemon injects the live engine so this package never imports kernel/pulse.
type PulseController interface {
	StatusMap() map[string]any
	Pause()
	Resume()
	Beat()
	SetCadence(d time.Duration) time.Duration
	SetDial(dial string) string
	SetQuietHours(spec string) string
	FlushDigest() int
	RemoveObserver(name string) int
	PendingAsks() []map[string]any
	ResolveAsk(issueKey string, approve bool) (found, acted bool)
}

// SetPulse wires the live engine. Safe to call once after construction,
// before Start. Nil leaves Pulse reported as disabled.
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

// PulseObservers is the narrow slice of the pulse engine's observer registry the
// control plane needs to add watches at runtime: disk-space watches (M767) and
// warden-gated command probes (M768). The daemon injects an adapter over the
// live engine (it owns the DiskUsage func, the warden and the state store), so
// this package never imports kernel/pulse. Nil when pulse is disabled; the
// handlers report the watch commands as unavailable.
type PulseObservers interface {
	// AddDiskObserver registers a disk-space observer for path that alerts when
	// free space drops below minPct. Returns the observer name + ok.
	AddDiskObserver(path string, minPct float64) (string, bool)
	// AddProbeObserver registers a command-probe observer: argv runs each beat
	// (warden-gated) and alerts on red↔green transitions. Returns the observer
	// name + ok.
	AddProbeObserver(name string, argv []string) (string, bool)
}

// SetPulseObservers wires the runtime observer registry (disk watches + command
// probes) in one step. Nil leaves both watch commands reporting unavailable.
func (s *Server) SetPulseObservers(o PulseObservers) { s.observers = o }

// funcObservers adapts the legacy per-func setters (SetDiskWatch/SetProbeWatch)
// onto PulseObservers. Each capability keeps its own independent nil state, so a
// server wired with only one of the two funcs reports the other command
// unavailable — exactly the pre-interface behavior.
type funcObservers struct {
	disk  func(path string, minPct float64) (string, bool)
	probe func(name string, argv []string) (string, bool)
}

func (f *funcObservers) AddDiskObserver(path string, minPct float64) (string, bool) {
	if f.disk == nil {
		return "", false
	}
	return f.disk(path, minPct)
}

func (f *funcObservers) AddProbeObserver(name string, argv []string) (string, bool) {
	if f.probe == nil {
		return "", false
	}
	return f.probe(name, argv)
}

// legacyObservers returns the funcObservers shim currently installed, creating
// one (replacing any non-shim adapter) when needed.
func (s *Server) legacyObservers() *funcObservers {
	if f, ok := s.observers.(*funcObservers); ok {
		return f
	}
	f := &funcObservers{}
	s.observers = f
	return f
}

// diskWatchAvailable reports whether the runtime disk-watch path is wired —
// per-capability, so a legacy shim with only the probe func set still reports
// disk watches unavailable.
func (s *Server) diskWatchAvailable() bool {
	if f, ok := s.observers.(*funcObservers); ok {
		return f.disk != nil
	}
	return s.observers != nil
}

// probeWatchAvailable reports whether the runtime command-probe path is wired.
func (s *Server) probeWatchAvailable() bool {
	if f, ok := s.observers.(*funcObservers); ok {
		return f.probe != nil
	}
	return s.observers != nil
}

// SetDiskWatch wires the runtime disk-watch path (M767). The daemon injects a closure
// that builds a pulse disk observer (with its DiskUsage func) and registers it on the
// engine, so the control plane stays decoupled from kernel/pulse. Kept for
// compatibility; new wiring should inject a PulseObservers via SetPulseObservers
// (or Bind).
func (s *Server) SetDiskWatch(fn func(path string, minPct float64) (string, bool)) {
	s.legacyObservers().disk = fn
}

// handlePulseWatch adds a disk-space watch to the proactive heartbeat at runtime
// (M767): the agent will alert when free space on `path` drops below `min_pct`. The
// new observer takes effect on the next beat.
func (s *Server) handlePulseWatch(conn net.Conn, req Request) {
	if !s.diskWatchAvailable() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "watches are unavailable (pulse is disabled)"})
		return
	}
	path, err := requiredArgString(req.Args, "path")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var pct float64
	switch v := req.Args["min_pct"].(type) {
	case float64:
		pct = v
	case string:
		pct, _ = strconv.ParseFloat(v, 64)
	}
	if pct <= 0 || pct >= 100 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.min_pct must be between 0 and 100"})
		return
	}
	name, ok := s.observers.AddDiskObserver(path, pct)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "could not add the watch"})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"added": true, "observer": name}})
}

// SetProbeWatch wires the runtime command-probe path (M768). The daemon injects a
// closure that builds a warden-gated probe observer and registers it on the engine.
// Kept for compatibility; new wiring should inject a PulseObservers via
// SetPulseObservers (or Bind).
func (s *Server) SetProbeWatch(fn func(name string, argv []string) (string, bool)) {
	s.legacyObservers().probe = fn
}

// handlePulseProbe adds a command-probe watch to the heartbeat at runtime (M768): the
// agent runs `command` each beat and alerts when its pass/fail flips (e.g. watch CI or
// a build). The command runs through the warden, like any agent shell call.
func (s *Server) handlePulseProbe(conn net.Conn, req Request) {
	if !s.probeWatchAvailable() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "watches are unavailable (pulse is disabled)"})
		return
	}
	sa, err := argStrings(req.Args, "name", "command")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	name, command := sa["name"], sa["command"]
	argv := strings.Fields(command)
	if name == "" || len(argv) == 0 {
		s.failMsg(conn, req, "args.name and args.command required")
		return
	}
	obs, ok := s.observers.AddProbeObserver(name, argv)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "could not add the probe"})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"added": true, "observer": obs}})
}

// handlePulseUnwatch removes runtime-added watches by observer name (M769) — the
// inverse of handlePulseWatch/handlePulseProbe. Startup observers (self:health and any
// AGEZT_PULSE_* probes) are never removed; the engine only drops observers it was given
// via AddObserver. Returns how many were dropped (0 if the name matched nothing
// removable). Takes effect on the next beat.
func (s *Server) handlePulseUnwatch(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	name, err := requiredArgString(req.Args, "name")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removed := s.pulse.RemoveObserver(name)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed}})
}

// handlePulseDial changes the proactivity dial live (M757/M758): quiet/balanced/chatty.
// An unknown value is normalized to balanced. Returns the applied dial.
func (s *Server) handlePulseDial(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	dial, _, err := argString(req.Args, "dial")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	applied := s.pulse.SetDial(dial)
	s.persistPulseSetting("AGEZT_PULSE_DIAL", applied)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"dial": applied}})
}

// handlePulseQuiet sets the quiet-hours window live (M770): during it, only alert/act
// briefs break through, regardless of the dial. hours is the "START-END" 24h form
// (e.g. "22-7"); an empty or invalid value disables quiet hours. Persisted so it
// survives restart (buildPulse reads AGEZT_PULSE_QUIET_HOURS). Returns the applied spec.
func (s *Server) handlePulseQuiet(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	hours, _, err := argString(req.Args, "hours")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	applied := s.pulse.SetQuietHours(hours)
	s.persistPulseSetting("AGEZT_PULSE_QUIET_HOURS", applied)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"quiet": applied}})
}

// handlePulseFlush delivers any held digest items immediately (M761) instead of
// waiting for the periodic flush. Returns how many items were flushed.
func (s *Server) handlePulseFlush(conn net.Conn, req Request) {
	if s.pulse == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "pulse is disabled (AGEZT_PULSE=off)"})
		return
	}
	n := s.pulse.FlushDigest()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"flushed": n}})
}

// registerPulseControlCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
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
