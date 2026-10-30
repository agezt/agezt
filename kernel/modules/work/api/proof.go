// SPDX-License-Identifier: MIT

package api

// The work module's public vocabulary for the PROOF CYCLE: the two narrow
// ways to ASK the host about the evidence a proof rests on.
//
// The proof IS the board's business — "the durable, checkable evidence that a
// workboard task actually satisfied its acceptance criteria" — so the board's
// service carries the verb and the board's dependencies carry the ports. This
// file holds the two shapes those ports take.
//
// It exists because another module may import a module's api package and
// nothing else (archcheck's principle P5). The runtime keeps its own names as
// forwarders, so the host's public API is unchanged while the implementation
// lives in the module.
//
// The ROWS live in the contract layer (kernel/contract/proofapi and
// kernel/contract/assureapi): the agent-facing tools that expose them are
// plugins, and a plugin may reach the contract and the platform layers only.
//
// Rules of the road:
//
//   - Types, interfaces and ports only. No logic that needs a store, a bus or
//     a clock — those are the implementation package's business.
//   - One row, one home. A row both the host and the module name is declared
//     exactly once — in the contract package — and every other side aliases
//     it.

import (
	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/event"
)

// EventScanner is the host's answer to "walk the durable events".
//
// The proof needs the journal RANGE its task's events fall into, and the rule
// that computes that range — scan by correlation id, not by a fixed tail
// window — belongs to the proof, not to the host. So the module holds a way to
// ASK and does the walking itself.
//
// The host's journal satisfies this port structurally; nothing here names the
// host's journal package, so the module never reaches it.
type EventScanner interface {
	// Range visits every durable event. The visit function's error stops the
	// walk and is returned.
	Range(visit func(*event.Event) error) error
}

// ArtifactLister is the host's answer to "which artifacts were recorded".
//
// The proof lists the artifacts produced under one correlation id, and that
// choice — under WHICH correlation — is the proof's rule. The module holds a
// way to ASK and filters itself.
type ArtifactLister interface {
	// List returns the entries matching f, newest first.
	List(f remoteapi.Filter) []remoteapi.Artifact
}
