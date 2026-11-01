// SPDX-License-Identifier: MIT

package delegation

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/prompt"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
)

// Host is the daemon's surface the delegation engine asks for. Every method
// is a door the daemon opens; the module never reaches into the daemon's
// fields. The daemon's implementation lives in kernel/runtime/subagent_doors.go.
//
// The split of responsibility is deliberate: the module owns the POLICY
// (who may delegate, under which limits, with which identity) and the daemon
// owns the MECHANISM (how a child run is configured and executed). That is why
// BaseLoopConfig and RunChild are doors and not module code: the child's loop
// configuration is the next family to move (kernel/runtime/loopconfig.go) and
// the loop itself the one after (kernel/agent).
type Host interface {
	// RosterGet resolves a roster slug to its profile. The second result is
	// false when no such agent exists.
	RosterGet(ref string) (agentapi.Profile, bool)
	// ProfileModel returns the profile's effective model: its own model
	// when it has one, otherwise its AGEZT_MODEL config override. The
	// override vocabulary belongs to the daemon's configuration, so the
	// daemon reads it.
	ProfileModel(prof agentapi.Profile) string
	// Model returns the LIVE default model id (hot-swapped on provider
	// reload) — not the boot seed.
	Model() string
	// System returns the LIVE default identity (editable at runtime).
	System() string
	// Halted reports whether the daemon is halted.
	Halted() bool
	// HaltedError returns the daemon's halt error, so a halted delegation
	// fails with the SAME error identity the daemon used before the move
	// (errors.Is against runtime.ErrHalted still holds).
	HaltedError() error
	// NewCorrelation mints a fresh correlation id for a child run.
	NewCorrelation() string
	// Publish publishes an event on the daemon's bus.
	Publish(spec event.Spec) (*event.Event, error)
	// RangeJournal walks the journal in order. The module uses it to sum
	// the spend of a delegation tree.
	RangeJournal(fn func(*event.Event) error) error
	// RegisterSteer publishes a child's live-steering control surface in
	// the daemon's shared steering registry, so an operator can pause,
	// step or steer an INDIVIDUAL sub-agent from the cockpit.
	RegisterSteer(corr string, rc *steer.Control)
	// UnregisterSteer removes a child's steering control surface.
	UnregisterSteer(corr string)
	// RecordSkillOutcome reports a child run's outcome for the skills it
	// activated, so the skill forge can learn from it.
	RecordSkillOutcome(corr string, skillIDs []string, ok bool)
	// BaseLoopConfig returns the child's base loop configuration — the
	// daemon's shared governance/capacity base, identical to a root run's.
	BaseLoopConfig(ctx context.Context, corr, model string) agent.LoopConfig
	// CompleteChildLifecycle advances the durable lifecycle of a finished
	// child run. The daemon owns the run lifecycle; the engine owns the
	// delegation.
	CompleteChildLifecycle(ctx context.Context, corr string)
}

// Deps carries the module's collaborators that are not daemon doors.
type Deps struct {
	// Host is the daemon's door surface. Required.
	Host Host
	// Config returns the live delegation policy. Required.
	Config func() Config
	// RunChild runs a prepared child's loop to completion and returns its
	// answer. The daemon supplies it: the module assembles the loop
	// configuration (the policy half) and the daemon executes it (the
	// mechanism half). Required.
	RunChild func(ctx context.Context, lc agent.LoopConfig, task string) (string, error)
	// Prompt is the runs module's prompt service: it assembles the
	// sub-agent's system prompt and reports the skills it activated.
	// Required.
	Prompt *prompt.Service
	// Registry is the runs module's live-run registry. The module registers
	// an async child in it, so Halt and CancelRun reach the child exactly
	// as they reach a root run. Required for async delegation; a nil
	// registry makes an async spawn fail with a clear error.
	Registry *registry.Registry
	// Sleep waits for the retry backoff. nil means "wait on a timer".
	Sleep func(ctx context.Context, d time.Duration) error
}

// sleep waits for the retry backoff, honouring ctx.
func (d Deps) sleep(ctx context.Context, delay time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, delay)
	}
	if delay <= 0 {
		return nil
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
