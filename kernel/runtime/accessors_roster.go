// SPDX-License-Identifier: MIT

package runtime

// Provenance: Kernel accessors: roster CRUD
//             (Roster/AddProfile/SetProfileEnabled/SetProfileRetired/AgentImpact/UpdateProfile/RemoveProfile).
//             Code extracted from accessors.go during the Day-56 god-file split.
//             Public API unchanged.
//
// W3.76n: the roster's WRITES are the agents module's. The module keeps the
// lifecycle, the events and the guardian rule; the daemon keeps the names and
// forwards. The READS stayed here: they are the host's nil-safe view over the
// contract's port (the shape W3.58 established), and a read over a port is
// the host's face.

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/contract/agentapi"
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

// AddProfile validates and persists a named agent profile, journaling
// roster.created so the agent's birth is auditable.
//
// The write is the MODULE's: the daemon forwards and hands back the answer.
func (k *Kernel) AddProfile(p agentapi.Profile) (agentapi.Profile, error) {
	svc := k.rosterService()
	if svc == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return svc.Add(p)
}

// SetProfileEnabled pauses/resumes an agent profile, journaling roster.updated.
func (k *Kernel) SetProfileEnabled(ref string, enabled bool) (agentapi.Profile, error) {
	svc := k.rosterService()
	if svc == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return svc.SetEnabled(ref, enabled)
}

// SetProfileRetired moves an agent to the graveyard (true) or revives it (false)
// by ref, journaling roster.updated. Retiring also pauses the agent so it stops
// firing (M846). A graveyard agent is excluded from delegation (runSubAgent).
func (k *Kernel) SetProfileRetired(ref string, retired bool, reason ...string) (agentapi.Profile, error) {
	svc := k.rosterService()
	if svc == nil {
		return agentapi.Profile{}, agentapi.ErrNoRoster
	}
	return svc.SetRetired(ref, retired, reason...)
}

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

// UpdateProfile edits a profile's mutable fields via mutate, journaling
// roster.updated (action "edited"). Identity/lifecycle fields are protected by
// the store. Returns false + nil error for an unknown ref (standing pattern).
func (k *Kernel) UpdateProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	svc := k.rosterService()
	if svc == nil {
		return agentapi.Profile{}, false, agentapi.ErrNoRoster
	}
	return svc.Update(ref, mutate)
}

// RemoveProfile deletes an agent profile, journaling roster.removed when it
// existed. Returns whether it existed.
//
// A shipped guardian (System) is protected from a hard delete (M961) — that
// rule is the MODULE's now: the roster knows which of its rows are the
// daemon's own self-healing fleet.
func (k *Kernel) RemoveProfile(ref string) (bool, error) {
	svc := k.rosterService()
	if svc == nil {
		return false, agentapi.ErrNoRoster
	}
	return svc.Remove(ref)
}
