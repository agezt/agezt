// SPDX-License-Identifier: MIT

package api

// The agents module's public vocabulary for the ROSTER: the profile writes
// the host drives, and the host's ports the module needs to answer them.
//
// The roster is the agents module's (the target architecture's module
// catalogue says so in one line: the `agents` module owns "roster profiles,
// guardians"). Its DATA was already here — the store, the normalization and
// the validation live in kernel/modules/agents/roster — and its OPERATIONS
// lived in the daemon (kernel/runtime/accessors_roster.go), which held the
// store, applied the write, published the event and knew that a shipped
// guardian may be paused or retired but never removed.
//
// A module that keeps the data and gives away the meaning of the data is the
// split this program removes. So the WRITES moved down: the module keeps the
// lifecycle, and the daemon keeps its names — AddProfile, UpdateProfile,
// RemoveProfile, SetProfileEnabled and SetProfileRetired are still the
// daemon's verbs, and every caller outside the runtime is untouched.
//
// The READS stayed with the host (the nil-safe view over the contract's port,
// the shape W3.58 established): the store port is the host's file, and a read
// over a port is the host's face.
//
// The store port stays the CONTRACT's host port (agentapi.Store): the file it
// lives in is the host's business, and the host may not name the module that
// keeps it. The service is built over that port by the composition root,
// which is the one place that knows both.

import (
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
)

// RosterService is the roster's public face: the profile lifecycle the host
// drives.
//
// The verbs are the ROSTER's own, not the daemon's: a host that holds a
// service can add an agent, edit it, pause it, retire it and remove it, and
// it can never reach past the roster into the daemon's machinery.
//
// Every verb answers with the profile it touched (or with whether it was
// there at all), so a caller never reads the store to learn what happened.
type RosterService interface {
	// Add persists a new profile and announces its birth.
	Add(p agentapi.Profile) (agentapi.Profile, error)
	// Update edits a profile's mutable fields via mutate. The bool is false
	// for an unknown ref (the standing pattern: no error, no row).
	Update(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	// Remove deletes a profile; the bool says whether it existed.
	//
	// A shipped guardian (a system profile) is refused: the daemon's own
	// self-healing fleet may be paused or retired, never removed.
	Remove(ref string) (bool, error)
	// SetEnabled pauses (false) or resumes (true) a profile.
	SetEnabled(ref string, enabled bool) (agentapi.Profile, error)
	// SetRetired moves a profile to the graveyard (true) or revives it
	// (false).
	SetRetired(ref string, retired bool, reason ...string) (agentapi.Profile, error)
}

// RosterDependencies are the host's ports the roster needs to answer a write.
//
// Every port is a way to ASK, never a way to reach: the roster publishes on
// the host's bus and tells the host that an agent changed, and it holds
// nothing else of the host's.
type RosterDependencies struct {
	// Store is the contract's host port over the roster's file.
	//
	// A nil store means the host opened no roster, and every write refuses
	// with the contract's row (agentapi.ErrNoRoster).
	Store agentapi.Store

	// Publish emits one event on the host's bus.
	//
	// The host owns the bus; the roster only says what happened to an agent,
	// and the subject and the kind are the roster's own. A nil port means the
	// host has no bus, and the roster stays quiet rather than publishing into
	// nothing.
	Publish func(spec event.Spec)
}

// RosterServiceBuilder builds the roster's service over the dependencies the
// host collected.
//
// The daemon hands over the BUILDER (it names the module); the host calls it
// with the store and its own publisher — so every roster change journals
// through the host's bus without either side naming the other's
// implementation.
type RosterServiceBuilder func(RosterDependencies) RosterService
