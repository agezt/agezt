// SPDX-License-Identifier: MIT

package runtime

import (
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
)

// The PROMPT ASSEMBLY is the RUNS MODULE's (kernel/modules/runs/prompt,
// W3.76e): the layer that decides what a run knows before it starts — the
// live identity, the operator profile, the taste exemplars, the recalled
// memory, the resolved world entities and the activated skills — plus the
// host-environment preamble that tells the model which machine it is on.
//
// The daemon holds the SERVICE and the host port that feeds it, and nothing
// else: the run engine asks the module for its own prompt (W3.76af), and the
// delegation engine — which lives in the same module — has always held the
// very same service. The daemon's own use of it is one call: the
// context-selection manifest it derives from the run's journal
// (context_selection.go).
//
// The file is named prompt_host.go, NOT prompt.go: the daemon's prompt.go was
// the run context's profile renderer, and it moved to the runs module in
// W3.76c (kernel/modules/runs/runctx/profile.go). The ratchet in
// runctx_ratchet_test.go watches that name, and it should keep watching it:
// "the daemon has no prompt.go" is a true statement about the run context's
// home, whatever else the daemon's prompt files are called.
//
// The module reaches the daemon through a THREE-verb Host port — the live
// identity, the bus and the clock — and reads the four stores through the
// contract rows the daemon already holds.

// promptHost is the daemon's side of the module's Host port.
type promptHost struct{ k *Kernel }

// System is the daemon's LIVE default identity (M710): what it is right now,
// editable at runtime without a restart.
func (h promptHost) System() string { return h.k.System() }

// Publish journals an event through the daemon's bus: the taste injection
// and every context-selection manifest travel through it.
func (h promptHost) Publish(spec event.Spec) (*event.Event, error) { return h.k.bus.Publish(spec) }

// Now is the daemon's clock. The module asks rather than reading the wall
// clock itself, so the preamble's date and the candidates' freshness cannot
// drift apart.
func (h promptHost) Now() time.Time { return time.Now() }

// promptConfig is the SNAPSHOT of the injection settings the module
// assembles a prompt with. The daemon's Config is read and never written,
// so a snapshot cannot drift from the daemon's own view of it.
func promptConfig(cfg Config) prompt.Config {
	return prompt.Config{
		ProfileInject:     cfg.ProfileInject,
		TasteInject:       cfg.TasteInject,
		TasteTopK:         cfg.TasteTopK,
		MemoryInject:      cfg.MemoryInject,
		MemoryTopK:        cfg.MemoryTopK,
		WorldInject:       cfg.WorldInject,
		WorldTopK:         cfg.WorldTopK,
		SkillInject:       cfg.SkillInject,
		SkillTopK:         cfg.SkillTopK,
		EnvironmentInject: cfg.EnvironmentInject,
		WorkspaceRoot:     cfg.WorkspaceRoot,
	}
}
