// SPDX-License-Identifier: MIT

package agentapi

// Provenance: the roster's STORE port, its sentinels and the narrow read port
//             (W3.56). The profile row was already in the contract (W3.14);
//             what the layers above the module could not reach was the store
//             BEHIND it — the module's concrete file-backed type, the two
//             sentinels they compare against, and the repair surface they
//             inject.

import "errors"

// ErrNotFound is returned for an unknown profile id/slug.
//
// The sentinel lives in the contract because the layers above the module
// compare against it: a module may not be named by the layers above it, so
// the error the layers above it compare against must not live in the module
// (the standing-order slice moved its sentinel for the same reason, W3.50).
var ErrNotFound = errors.New("roster: profile not found")

// ErrRetired is returned when a caller tries to resume a graveyard agent through
// the pause/resume lifecycle. Graveyard exit is a distinct revive transition.
//
// The sentinel lives in the contract for the same reason as ErrNotFound.
var ErrRetired = errors.New("roster: profile is retired")

// Store is the roster's file-backed surface: the named agent profiles and the
// lifecycle transitions over them.
//
// It is the module's WHOLE public store surface — eight verbs, no more. The
// test seam (SetNowForTest) is deliberately NOT on the port: a port is what
// the layers above the module may call, and a clock hook is not theirs.
//
// No verb takes a time.Time: the store stamps the rows from its own clock,
// so the contract layer never has to name one (principle P4).
type Store interface {
	// Add persists a new profile and answers it.
	Add(p Profile) (Profile, error)
	// SetEnabled pauses (false) or resumes (true) a profile.
	SetEnabled(ref string, enabled bool) (Profile, error)
	// SetRetired moves a profile to the graveyard (true) or revives it (false).
	SetRetired(ref string, retired bool, reason ...string) (Profile, error)
	// Update edits a profile's mutable fields via mutate and answers it.
	Update(ref string, mutate func(*Profile)) (Profile, error)
	// Remove deletes a profile and answers whether it existed.
	Remove(ref string) (Profile, bool, error)
	// Get returns the profile named by ref; the bool is false if absent.
	Get(ref string) (Profile, bool)
	// List returns every profile.
	List() []Profile
	// Count returns the number of profiles.
	Count() int
}

// Reader is the read-only half of the roster — what a consumer that only
// looks agents up needs (the agent gateway, the web UI's agent list).
//
// It is a NARROWER port on purpose: a reader that could also write would
// hand every read-only consumer the whole lifecycle, and the gateway's
// handlers have no business mutating a profile.
type Reader interface {
	// Get returns the profile named by ref; the bool is false if absent.
	Get(ref string) (Profile, bool)
	// List returns every profile.
	List() []Profile
}

// RepairSource is the agent-control surface a repair pass drives. The daemon
// injects the implementation; nothing that drives a repair imports the tool
// that provides it.
//
// It is one verb: a repair pass is a governed run of the target agent that
// may change its durable profile and reports what it changed.
type RepairSource interface {
	// RepairAgent runs a governed self-repair pass AS the target agent,
	// optionally auto-applying a closing profile proposal from the run's
	// final answer.
	RepairAgent(ref, reason string) (RepairResult, error)
}
