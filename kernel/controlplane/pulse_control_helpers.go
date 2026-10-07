// SPDX-License-Identifier: MIT
package controlplane

import (
	apppulse "github.com/agezt/agezt/kernel/app/pulse"
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
type PulseController = apppulse.Controller

// PulseObservers is the narrow slice of the pulse engine's observer registry the
// control plane needs to add watches at runtime: disk-space watches (M767) and
// warden-gated command probes (M768). The daemon injects an adapter over the
// live engine (it owns the DiskUsage func, the warden and the state store), so
// this package never imports kernel/pulse. Nil when pulse is disabled; the
// handlers report the watch commands as unavailable.
type PulseObservers = apppulse.Observers

// SetPulseObservers wires the runtime observer registry (disk watches + command
// probes) in one step. Nil leaves both watch commands reporting unavailable.
func (s *Server) SetPulseObservers(o PulseObservers) { s.observers = o }

func (s *Server) pulseControls() *apppulse.Controls {
	return apppulse.NewControls(s.pulse, s.observers, s.persistPulseSetting)
}
