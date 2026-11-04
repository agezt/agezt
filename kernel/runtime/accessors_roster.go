// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: roster CRUD
//             (Roster/RosterGet/RosterList/AgentImpact/RosterService).
//             Code extracted from accessors.go during the Day-56 god-file split.
//             Public API unchanged.
//
// W3.76n: the roster's WRITES are the agents module's. The module keeps the
// lifecycle, the events and the guardian rule; the daemon keeps the names and
// forwards. The READS stayed here: they are the host's nil-safe view over the
// contract's port (the shape W3.58 established), and a read over a port is
// the host's face.
//
// W3.76y: the daemon holds NO verb per roster operation any more. The module's
// service already answered the contract's names (Add, SetEnabled, SetRetired,
// Update, Remove), so the daemon's five forwarders (AddProfile, UpdateProfile,
// RemoveProfile, SetProfileEnabled, SetProfileRetired) were a THIRD spelling of
// one family. The daemon hands over the module's FACE now, and the READS stay:
// they are the host's nil-safe view, and a read over a port is the host's.

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	agentsapi "github.com/agezt/agezt/kernel/modules/agents/api"
)

// Roster returns the roster store port.
//
// The roster is the DAEMON's file (W3.56): the kernel holds the contract's
// port, and the daemon opens roster/roster.json and hands it over. A host
// that was given none holds a NIL port — the reads below answer "no agents"
// and the writes refuse, rather than calling into nothing.
func (k *Kernel) Roster() agentapi.Store { return k.roster }

// RosterGet returns the profile named by ref.
//
// It is the NIL-SAFE read: the roster is the agent list, and a kernel with no
// roster has no agents — not a panic. Every read above the module goes
// through here, so the missing-roster case is answered ONCE, here.
func (k *Kernel) RosterGet(ref string) (agentapi.Profile, bool) {
	if k.roster == nil {
		return agentapi.Profile{}, false
	}
	return k.roster.Get(ref)
}

// RosterList returns every profile.
//
// The NIL-SAFE read: a kernel with no roster has no agents.
func (k *Kernel) RosterList() []agentapi.Profile {
	if k.roster == nil {
		return nil
	}
	return k.roster.List()
}

// RosterService returns the AGENTS MODULE's roster service — the roster's
// lifecycle.
//
// The daemon holds no verb per roster operation any more (W3.76y): the module
// keeps the lifecycle, the events and the guardian rule, and the daemon hands
// over the module's FACE. The verbs are the CONTRACT's names (Add, SetEnabled,
// SetRetired, Update, Remove), so one operation has one name.
//
// The face is NIL when the host holds no roster — a daemon that was given
// neither the file nor the module's builder holds nothing, and the callers
// above refuse in its place (the contract's row: agentapi.ErrNoRoster) rather
// than calling into nothing.
func (k *Kernel) RosterService() agentsapi.RosterService { return k.rosterService() }

// AgentImpact reports what depends on an agent before it is retired/removed
// (M846) — the standing orders that fire AS it. The operator sees this in the
// retire confirmation so the "etkileri" are explicit, not a surprise. Returns the
// affected orders as "name (id)" strings, or nil when nothing references it.
//
// This read stayed with the host (W3.76n): the standing orders are ANOTHER
// domain's rows, and a module may not name another module's data. The roster
// module knows the roster; the host knows what stands behind it.
func (k *Kernel) AgentImpact(slug string) []string {
	slug = strings.TrimSpace(slug)
	if slug == "" || k.standing == nil {
		return nil
	}
	var out []string
	for _, o := range k.standing.List() {
		if strings.EqualFold(strings.TrimSpace(o.Agent), slug) {
			name := o.Name
			if name == "" {
				name = o.ID
			}
			out = append(out, fmt.Sprintf("%s (%s)", name, o.ID))
		}
	}
	return out
}
