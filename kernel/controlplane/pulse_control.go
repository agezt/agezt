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

// SetDiskWatch wires the runtime disk-watch path (M767). The daemon injects a closure
// that builds a pulse disk observer (with its DiskUsage func) and registers it on the
// engine, so the control plane stays decoupled from kernel/pulse.
func (s *Server) SetDiskWatch(fn func(path string, minPct float64) (string, bool)) { s.diskWatch = fn }

// handlePulseWatch adds a disk-space watch to the proactive heartbeat at runtime
// (M767): the agent will alert when free space on `path` drops below `min_pct`. The
// new observer takes effect on the next beat.
func (s *Server) handlePulseWatch(conn net.Conn, req Request) {
	if s.diskWatch == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "watches are unavailable (pulse is disabled)"})
		return
	}
	path, _ := req.Args["path"].(string)
	if path == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.path required"})
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
	name, ok := s.diskWatch(path, pct)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "could not add the watch"})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"added": true, "observer": name}})
}

// SetProbeWatch wires the runtime command-probe path (M768). The daemon injects a
// closure that builds a warden-gated probe observer and registers it on the engine.
func (s *Server) SetProbeWatch(fn func(name string, argv []string) (string, bool)) { s.probeWatch = fn }

// handlePulseProbe adds a command-probe watch to the heartbeat at runtime (M768): the
// agent runs `command` each beat and alerts when its pass/fail flips (e.g. watch CI or
// a build). The command runs through the warden, like any agent shell call.
func (s *Server) handlePulseProbe(conn net.Conn, req Request) {
	if s.probeWatch == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "watches are unavailable (pulse is disabled)"})
		return
	}
	name, _ := req.Args["name"].(string)
	command, _ := req.Args["command"].(string)
	argv := strings.Fields(command)
	if name == "" || len(argv) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name and args.command required"})
		return
	}
	obs, ok := s.probeWatch(name, argv)
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
	name, _ := req.Args["name"].(string)
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
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
	dial, _ := req.Args["dial"].(string)
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
	hours, _ := req.Args["hours"].(string)
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
