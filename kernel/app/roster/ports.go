// SPDX-License-Identifier: MIT

package roster

import (
	"github.com/agezt/agezt/kernel/contract/agentapi"
	agentsapi "github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/runtime"
)

// The roster's profile ports, filled from the kernel handle.
//
// The APPLICATION owns this: it knows which capabilities the roster needs and
// how each of them answers. The host hands over the handle and nothing else —
// it does not decide what the roster reads, and it cannot change the roster's
// wiring without editing the application.
//
// The same shape already exists in app/council, app/catalog, app/providers and
// app/runs: the application takes the kernel and builds its own ports.
//
// The rows the roster manages are CONTRACT rows (agentapi.Profile is the
// agents module's row, which is the contract's), so the port hands them over
// without a conversion.

// ProfilePorts are the roster's capabilities over the agent profiles it
// manages.
//
// Every host that fills these ports asks the same eight questions — who is
// this agent, who are all the agents, add one, edit one, remove one, pause
// one, retire one, and what depends on one — so the roster asks them once,
// here, and the host binds them to the kernel.
type ProfilePorts struct {
	Get        func(ref string) (agentapi.Profile, bool)
	List       func() []agentapi.Profile
	Add        func(p agentapi.Profile) (agentapi.Profile, error)
	Update     func(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	Remove     func(ref string) (bool, error)
	SetEnabled func(ref string, enabled bool) (agentapi.Profile, error)
	SetRetired func(ref string, retired bool, reason ...string) (agentapi.Profile, error)
	Impact     func(slug string) []string
}

// ProfilePortsFromKernel fills the roster's profile ports from the kernel
// handle.
//
// The WRITES go to the AGENTS MODULE's service — the roster's lifecycle is the
// module's (W3.76y), and the daemon hands over the module's FACE. The daemon
// used to forward five verbs of its own here (AddProfile, SetProfileEnabled,
// SetProfileRetired, UpdateProfile, RemoveProfile); they were a third spelling
// of one family, and one operation has one name.
//
// The face is nil when the host holds no roster: the writes then REFUSE with
// the contract's row (agentapi.ErrNoRoster) instead of calling into nothing.
// The READS stay with the host — they are the host's nil-safe view over the
// contract's port, and a read over a port is the host's.
func ProfilePortsFromKernel(k *runtime.Kernel) ProfilePorts {
	face := k.RosterService()
	return ProfilePorts{
		Get:  k.RosterGet,
		List: k.RosterList,
		Add:  func(p agentapi.Profile) (agentapi.Profile, error) { return rosterAdd(face, p) },
		Update: func(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
			return rosterUpdate(face, ref, mutate)
		},
		Remove: func(ref string) (bool, error) { return rosterRemove(face, ref) },
		SetEnabled: func(ref string, enabled bool) (agentapi.Profile, error) {
			return rosterSetEnabled(face, ref, enabled)
		},
		SetRetired: func(ref string, retired bool, reason ...string) (agentapi.Profile, error) {
			return rosterSetRetired(face, ref, retired, reason...)
		},
		Impact: k.AgentImpact,
	}
}

// The roster's writes, through the module's face.
//
// The face may be absent — a host with no roster holds no roster service — and
// a nil interface's method call is a panic. So every write asks the face
// first, and answers the contract's row when there is none.

// rosterAdd adds a profile through the module's face.
func rosterAdd(face agentsapi.RosterService, p agentapi.Profile) (agentapi.Profile, error) {
	if face == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return face.Add(p)
}

// rosterUpdate edits a profile through the module's face.
func rosterUpdate(face agentsapi.RosterService, ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	if face == nil {
		return agentapi.Profile{}, false, agentapi.ErrNoRoster
	}
	return face.Update(ref, mutate)
}

// rosterRemove removes a profile through the module's face.
func rosterRemove(face agentsapi.RosterService, ref string) (bool, error) {
	if face == nil {
		return false, agentapi.ErrNoRoster
	}
	return face.Remove(ref)
}

// rosterSetEnabled pauses or resumes a profile through the module's face.
func rosterSetEnabled(face agentsapi.RosterService, ref string, enabled bool) (agentapi.Profile, error) {
	if face == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return face.SetEnabled(ref, enabled)
}

// rosterSetRetired retires or revives a profile through the module's face.
func rosterSetRetired(face agentsapi.RosterService, ref string, retired bool, reason ...string) (agentapi.Profile, error) {
	if face == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return face.SetRetired(ref, retired, reason...)
}
