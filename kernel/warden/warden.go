// SPDX-License-Identifier: MIT

package warden

// Provenance: Warden types: Profile + IsKnown + constants. Code extracted from
//             warden.go during the Day-72 god-file split. Public API unchanged.

import (
	"sync"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/wardenapi"
)

// The rows live in the contract layer (kernel/contract/wardenapi): the tools
// that route a process launch through the warden are plugins, and a plugin
// may reach the contract and the platform — the row it names has to be one
// of the two. The aliases below keep every name this package's own callers
// use, so there is one row and one spelling of it.
type (
	// Profile is one of the four named isolation modes from SPEC-06 §2.
	Profile = wardenapi.Profile
	// Limits caps a single Run. Zero-value means "use the package default".
	Limits = wardenapi.Limits
	// Spec is one isolated execution request.
	Spec = wardenapi.Spec
	// Result is what a single Run produced.
	Result = wardenapi.Result
)

const (
	ProfileNone      = wardenapi.ProfileNone
	ProfileNamespace = wardenapi.ProfileNamespace
	ProfileContainer = wardenapi.ProfileContainer
	ProfileMicroVM   = wardenapi.ProfileMicroVM
)

// DefaultMaxOutputBytes caps captured stdout+stderr per Run. 256 KiB is
// large enough to carry a tool's full output even after a verbose run
// and small enough to keep one runaway invocation from blowing the
// journal budget. SPEC-02 §5 lists 256 KiB as the inline-attachment
// ceiling — we match it.
const DefaultMaxOutputBytes = wardenapi.DefaultMaxOutputBytes

// DefaultTimeout caps a single Run's wall time when no explicit
// timeout is provided.
const DefaultTimeout = wardenapi.DefaultTimeout

// DefaultWaitDelay bounds how long Wait() blocks for orphaned-child IO
// after the wrapper process is killed. Windows in particular needs this
// because killing cmd.exe does not reap its children.
const DefaultWaitDelay = wardenapi.DefaultWaitDelay

// Engine runs Specs and emits audit events to the bus.
//
// The two verbs a caller drives it with (Run, EffectiveProfile) are the
// contract's (kernel/contract/wardenapi) — a plugin names those and may not
// name this package. SetBus stays here: it names the bus, and the contract
// layer may not import it (archcheck's principle P4).
type Engine interface {
	wardenapi.Engine

	// SetBus attaches a bus post-construction. The daemon builds the
	// Warden before runtime.Open creates the kernel bus, so this lets
	// the wiring close the loop without circular-init gymnastics.
	// MUST be called before the first Run; the bus pointer is read
	// without a lock on the hot path.
	SetBus(b *bus.Bus)
}

// engine is the default cross-platform implementation. The Linux
// build-tag partition will eventually attach extra fields and override
// EffectiveProfile/Run for namespace+cgroups support.
type engine struct {
	bus       *bus.Bus
	container ContainerOptions

	// mu guards downgradeWarned so we only journal one downgrade event
	// per (requested-profile) per process lifetime — repeated tool
	// calls don't spam the journal with identical warnings.
	mu              sync.Mutex
	downgradeWarned map[Profile]struct{}
}

// New constructs the default engine. b may be nil for tests; events are
// silently dropped in that case.
