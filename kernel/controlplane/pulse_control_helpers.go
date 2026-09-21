// SPDX-License-Identifier: MIT

// pulse_control_helpers.go: persistPulseSetting + SetPulseObservers +
// watchesAvailable + handlePulseWatch/Probe/Unwatch/Dial/Quiet/Flush split off
// from pulse_control.go during the Day 211 god-file refactor (#147). Public API unchanged.
package controlplane

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/settings"
)

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
func (s *Server) watchesAvailable() bool { return s.observers != nil }

// handlePulseWatch adds a disk-space watch to the proactive heartbeat at runtime
// (M767): the agent will alert when free space on `path` drops below `min_pct`. The
// new observer takes effect on the next beat.
func (s *Server) handlePulseWatch(conn net.Conn, req Request) {
	if !s.watchesAvailable() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "watches are unavailable (pulse is disabled)"})
		return
	}
	path, err := requiredArgString(req.Args, "path")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// min_pct is a documented dual-type (number OR number-string) for
	// backwards compatibility with pre-typed-arg callers. Try the strict
	// numeric path first via argFloat64; fall back to argString +
	// strconv.ParseFloat when the value is a non-numeric JSON string.
	var pct float64
	if f, ok, err := argFloat64(req.Args, "min_pct"); err == nil && ok {
		pct = f
	} else if s, ok, _ := argString(req.Args, "min_pct"); ok {
		pct, _ = strconv.ParseFloat(s, 64)
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

// handlePulseProbe adds a command-probe watch to the heartbeat at runtime (M768): the
// agent runs `command` each beat and alerts when its pass/fail flips (e.g. watch CI or
// a build). The command runs through the warden, like any agent shell call.
func (s *Server) handlePulseProbe(conn net.Conn, req Request) {
	if !s.watchesAvailable() {
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
