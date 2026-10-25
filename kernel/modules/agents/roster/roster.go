// SPDX-License-Identifier: MIT

package roster

// Provenance: Profile types + Profile methods: Profile, RetryPolicy, HealthPolicy,
//             SelfRepairPolicy, NoisePolicy, AgentLifecycle, AgentTask, plus
//             Kind/AllowsDirectCall/AllowsDelegationFrom and safeCall. Code
//             extracted from roster.go during the Day-45 god-file split. Public API
//             unchanged.

import (
	"fmt"
	"regexp"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// ErrNotFound is returned for an unknown profile id/slug.
//
// The sentinel itself lives in the contract layer (kernel/contract/agentapi):
// the layers above the module compare against it, and they may not name this
// module. The name stays here as an alias, so `errors.Is` still matches it.
var ErrNotFound = agentapi.ErrNotFound

// ErrRetired is returned when a caller tries to resume a graveyard agent through
// the pause/resume lifecycle. Graveyard exit is a distinct revive transition.
//
// The sentinel itself lives in the contract layer (kernel/contract/agentapi).
var ErrRetired = agentapi.ErrRetired

// safeCall runs fn under a panic-recovery defer. If fn panics, the panic is caught
// and returned as an error so callers never receive a raw panic.
func safeCall(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return fn()
}

// Profile is one named agent identity. Slug is the address — unique,
// immutable, what operators and (future) other agents refer to it by.
//
// The row itself lives in the contract layer (kernel/contract/agentapi): the
// tools that steer the roster are plugins, and a plugin may only reach the
// contract. This package keeps the name, so every existing caller is unchanged.
type Profile = agentapi.Profile

// RetryPolicy is the agent's retry behaviour.
type RetryPolicy = agentapi.RetryPolicy

// HealthPolicy is the agent's health/staleness policy.
type HealthPolicy = agentapi.HealthPolicy

// SelfRepairPolicy is the agent's self-repair policy.
type SelfRepairPolicy = agentapi.SelfRepairPolicy

// NoisePolicy is the agent's noise policy.
type NoisePolicy = agentapi.NoisePolicy

// AgentLifecycle is the agent identity's lifecycle mode.
type AgentLifecycle = agentapi.AgentLifecycle

// AgentTask is one entry of the agent's durable task list.
type AgentTask = agentapi.AgentTask

// The lifecycle modes.
const (
	LifecyclePersistent       = agentapi.LifecyclePersistent
	LifecycleCycle            = agentapi.LifecycleCycle
	LifecycleRetireOnComplete = agentapi.LifecycleRetireOnComplete
)

// slugRe: lowercase, digit-or-letter first, then letters/digits/dot/dash/underscore.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var toolNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var configKeyRe = regexp.MustCompile(`^AGEZT_[A-Z0-9_]+$`)

const (
	maxSoulBytes       = 64 * 1024 // a soul is a prompt, not a novel
	maxFallbacks       = 8
	maxConfigOverrides = 128

	defaultSystemGuardianMaxCostMc         = 50_000_000 // $0.05/run
	defaultSystemGuardianMaxDailyMc        = 50_000_000 // $0.05/day
	defaultSystemGuardianNotifyCooldownSec = 8 * 3600
	defaultSystemGuardianMinNotifySeverity = "warning"
	defaultSystemGuardianTrustCeiling      = "L4"
	defaultSystemGuardianMemoryScopePrefix = "system/"
)

// Validate checks a profile's user-supplied fields (identity/lifecycle fields
// are kernel-assigned and not judged here).
