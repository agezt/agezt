// SPDX-License-Identifier: MIT

package roster

import (
	"github.com/agezt/agezt/kernel/contract/agentapi"
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
func ProfilePortsFromKernel(k *runtime.Kernel) ProfilePorts {
	return ProfilePorts{
		Get:        k.RosterGet,
		List:       k.RosterList,
		Add:        k.AddProfile,
		Update:     k.UpdateProfile,
		Remove:     k.RemoveProfile,
		SetEnabled: k.SetProfileEnabled,
		SetRetired: k.SetProfileRetired,
		Impact:     k.AgentImpact,
	}
}
