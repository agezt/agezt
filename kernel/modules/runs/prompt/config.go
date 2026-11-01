// SPDX-License-Identifier: MIT

package prompt

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/contract/skillapi"
	"github.com/agezt/agezt/kernel/contract/tasteapi"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/event"
)

// Config is the SNAPSHOT of the injection settings this module assembles a
// prompt with.
//
// It is a snapshot on purpose: the daemon's own Config is a value that is
// read, never written (the daemon's live settings live in their own fields),
// so the flags cannot drift between the composition root and the run. Every
// knob the prompt reads is here, and nothing else is.
type Config struct {
	// ProfileInject prepends the learned operator profile (M1000).
	ProfileInject bool
	// TasteInject prepends the curated exemplars ("what good looks like").
	TasteInject bool
	// TasteTopK is how many exemplars the taste store is asked for; zero
	// means three.
	TasteTopK int
	// MemoryInject prepends the recalled records.
	MemoryInject bool
	// MemoryTopK is how many records the memory is asked for; zero means five.
	MemoryTopK int
	// WorldInject prepends the resolved world entities.
	WorldInject bool
	// WorldTopK is how many entities the world model is asked for; zero
	// means five.
	WorldTopK int
	// SkillInject prepends the activated skills.
	SkillInject bool
	// SkillTopK is how many skills the curator is asked for; zero means three.
	SkillTopK int
	// EnvironmentInject turns the host-environment preamble on.
	EnvironmentInject bool
	// WorkspaceRoot is the directory the shell and file tools operate in;
	// it is stated in the preamble.
	WorkspaceRoot string
}

// Host is what this module needs from its host — and the list is short on
// purpose. The module assembles a prompt; it does not run the daemon, and it
// does not own the bus or the clock.
type Host interface {
	// System is the daemon's live default identity — who it is right now.
	// A run may override it for itself (the agent profile's soul).
	System() string
	// Publish journals an event: the taste injection and every context
	// selection manifest travel through it.
	Publish(spec event.Spec) (*event.Event, error)
	// Now is the host's clock. The module asks rather than reads the wall
	// clock, so a test can pin "now" and the preamble's date and the
	// candidates' freshness cannot drift apart.
	Now() time.Time
}

// Deps are the STORES this module reads. They are the contract rows the
// daemon already holds, handed over as ports: the module names the contract
// and never an implementation.
//
// Every one of them is OPTIONAL. A daemon that was given no memory holds a
// nil port, and a nil port answers "nothing to inject" rather than failing
// the run — the same answer the daemon's own nil-safe accessors give.
type Deps struct {
	// Memory is the memory module's manager: the operator profile, the
	// ranked records and the scoped recall.
	Memory memoryapi.Manager
	// Taste is the taste library: the curated exemplars for a scope.
	Taste tasteapi.Store
	// World is the world model's graph: the entities a phrase refers to.
	World worldmodelapi.Graph
	// Skills is the skill library's curator: the active skills that match
	// an intent, explicitly or by relevance.
	Skills skillapi.Curator
}
