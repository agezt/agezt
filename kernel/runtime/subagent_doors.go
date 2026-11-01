// SPDX-License-Identifier: MIT

package runtime

// Provenance: SPDX-License-Identifier: MIT The daemon's doors for the runs
//             module's delegation engine (kernel/modules/runs/delegation,
//             W3.76f). The engine's POLICY moved to the module in W3.76f; the
//             daemon keeps the MECHANISM — the live state the engine asks
//             for, and the child loop it runs.
//
// The file is named subagent_doors.go, not delegation.go: the daemon's
// delegation.go never existed, and the runctx ratchet from W3.76c watches
// the names the run context left behind.

import (
	"context"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/delegation"
	"github.com/agezt/agezt/kernel/modules/runs/steer"
)

// delegationHost is the daemon's door surface for the delegation engine.
type delegationHost struct{ k *Kernel }

// RosterGet resolves a roster slug to its profile.
func (h delegationHost) RosterGet(ref string) (agentapi.Profile, bool) {
	return h.k.RosterGet(ref)
}

// ProfileModel returns the profile's effective model: its own model when it
// has one, otherwise its AGEZT_MODEL config override. The daemon reads it:
// the override vocabulary belongs to the daemon's own configuration.
func (h delegationHost) ProfileModel(prof agentapi.Profile) string {
	if m := strings.TrimSpace(prof.Model); m != "" {
		return m
	}
	if override, ok := agentConfigOverrideRaw(prof.ConfigOverrides, "AGEZT_MODEL"); ok {
		if parsed, ok := agentConfigStringValue(override); ok {
			return parsed
		}
	}
	return ""
}

// Model is the daemon's LIVE default model id.
func (h delegationHost) Model() string { return h.k.Model() }

// System is the daemon's LIVE default identity.
func (h delegationHost) System() string { return h.k.System() }

// Halted reports whether the daemon is halted.
func (h delegationHost) Halted() bool { return h.k.IsHalted() }

// HaltedError is the daemon's own halt error, so a halted delegation fails
// with the same error identity the daemon used before the move.
func (h delegationHost) HaltedError() error { return ErrHalted }

// NewCorrelation mints a fresh correlation id for a child run.
func (h delegationHost) NewCorrelation() string { return h.k.NewCorrelation() }

// Publish journals an event through the daemon's bus.
func (h delegationHost) Publish(spec event.Spec) (*event.Event, error) {
	return h.k.bus.Publish(spec)
}

// RangeJournal walks the daemon's journal in order — the delegation engine
// reads it to sum a delegation tree's spend.
func (h delegationHost) RangeJournal(fn func(*event.Event) error) error {
	return h.k.journal.Range(fn)
}

// RegisterSteer publishes a child's live-steering control surface in the
// daemon's shared steering registry, so an operator can steer an INDIVIDUAL
// sub-agent (M631).
func (h delegationHost) RegisterSteer(corr string, rc *steer.Control) {
	h.k.steersMu.Lock()
	h.k.steers[corr] = rc
	h.k.steersMu.Unlock()
}

// UnregisterSteer removes a child's steering control surface.
func (h delegationHost) UnregisterSteer(corr string) {
	h.k.steersMu.Lock()
	delete(h.k.steers, corr)
	h.k.steersMu.Unlock()
}

// RecordSkillOutcome reports the child run's outcome for the skills it
// activated, so the skill forge can learn from it.
func (h delegationHost) RecordSkillOutcome(corr string, skillIDs []string, ok bool) {
	if h.k.forge != nil && len(skillIDs) > 0 {
		h.k.forge.RecordOutcome(corr, skillIDs, ok)
	}
}

// BaseLoopConfig is the child's base loop configuration: the daemon's shared
// governance/capacity base, identical to a root run's.
func (h delegationHost) BaseLoopConfig(ctx context.Context, corr, model string) agent.LoopConfig {
	return h.k.buildLoopConfig(ctx, corr, model)
}

// CompleteChildLifecycle advances the durable lifecycle of a finished child
// run.
func (h delegationHost) CompleteChildLifecycle(ctx context.Context, corr string) {
	h.k.CompleteAgentLifecycle(ctx, corr)
}

// delegationConfig is the LIVE delegation policy the engine reads on every
// spawn. The daemon's Config is read live — an operator raising a bound at
// runtime must see the change on the next delegation.
func delegationConfig(cfg Config) delegation.Config {
	return delegation.Config{
		MaxDepth:           cfg.SubAgentMaxDepth,
		MaxFanout:          cfg.SubAgentMaxFanout,
		MaxTotal:           cfg.SubAgentMaxTotal,
		MaxSpendMicrocents: cfg.SubAgentMaxSpendMicrocents,
		ModelAvailable:     cfg.ModelAvailable,
	}
}

// delegationRunChild runs a prepared child's loop to completion. The engine
// assembles the loop configuration (the policy half); the daemon executes it
// (the mechanism half) — the loop itself is the next family to move.
func delegationRunChild(ctx context.Context, lc agent.LoopConfig, task string) (string, error) {
	return agent.Run(ctx, lc, task)
}
