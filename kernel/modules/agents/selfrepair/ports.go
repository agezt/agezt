// SPDX-License-Identifier: MIT

package selfrepair

// The self-repair module's ports.
//
// Self-repair is a MODULE — the agents module's doctor — and a module may not
// reach into the host's kernel: it names the host's CAPABILITIES and the
// composition root hands them over. Until W5.4 the module held the whole
// kernel handle and asked it for eleven verbs, which was the last cross-module
// edge in the tree (the archcheck allowlist had one line left).
//
// Every capability now has ONE owner, and the port names that owner:
//
//	Bus          the host's event bus              (kernel/bus — L2 platform)
//	Journal      the host's durable journal        (a walk, not the store)
//	LiveProvider the host's live provider          (kernel/contract/llm — L1)
//	Scan         the agents module's reaper        (this module's own scanner)
//	Profile(s)   the roster's rows                 (kernel/contract/agentapi — L1)
//	Roster       the roster's lifecycle             (this module's own service)
//	Runner       the runs module's run engine      (kernel/modules/runs/api)
//	Correlation  the host's identity door           (the host mints the id)
//	BindProfile  the host's profile door            (kernel/runtime's door)
//	CapCost      the host's cost ceiling            (kernel/runtime's door)
//
// The last two are the DAEMON'S doors on purpose (kernel/runtime's
// accessors_ctx.go): they exist exactly for a module that may not name the
// runs module — "the profile and the ceiling — the self-repair module binds a
// run through them" — and the daemon's own ratchet pins them
// (kernel/runtime/runctx_ratchet_test.go). The module asks the host for the
// binding; it does not reach into the run context's vocabulary.
//
// Every port is OPTIONAL: a host that supplies none holds no healer, and the
// coordinator says so instead of calling into nothing. The three a WAKE cannot
// do without — the run engine, the correlation and the profile binding — are
// checked where they are used, and a wake without them REFUSES rather than
// running an agent with no profile.

import (
	"context"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	runsapi "github.com/agezt/agezt/kernel/modules/runs/api"
)

// Ports are the self-repair module's capabilities.
//
// The fields are the FILLING surface: the composition root (the daemon) hands
// the module each capability by name. The unexported readers below are the
// asking surface, and they answer for the host that supplied nothing.
type Ports struct {
	// Bus is the host's event bus: the coordinator publishes every phase of a
	// repair on it.
	Bus *bus.Bus

	// Journal walks the durable journal once. The coordinator reads the repair
	// history from it — the fingerprint the cooldown recognises and the
	// attempt count the profile's cap compares against.
	Journal func(func(*event.Event) error) error

	// LiveProvider is the host's live provider: the module asks it for the
	// current model chain of a task type (the routing rollback's evidence).
	//
	// It is a function and not a value because the provider is hot-swappable:
	// a chain captured at wire time would be a chain from the past.
	LiveProvider func() llm.Provider

	// Scan is the reaper's read-only fleet scan: the module's own detector,
	// injected by the composition root.
	//
	// It MAY answer with nothing: a daemon that was given no reaper module
	// holds no scanner, and the module reads that as an empty fleet.
	Scan func(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport

	// Profile reads the roster row named by ref. The bool says whether the
	// host holds such an agent.
	Profile func(string) (roster.Profile, bool)

	// Profiles lists the roster the scan judges.
	Profiles func() []roster.Profile

	// Roster is the roster's LIFECYCLE: the agents module's own service, and
	// the only way a repair may pause or retire an agent.
	//
	// A host that holds no roster holds no face, and the writes refuse with
	// the contract's row (agentapi.ErrNoRoster).
	Roster api.RosterService

	// Runner is the runs module's run engine. A module may name another
	// module's api package and nothing else (archcheck's principle P5), so
	// this is the runs module's published face and not its implementation.
	Runner runsapi.RunnerService

	// Correlation mints the correlation a wake runs under. The id is the
	// HOST's: the module asks for one, it does not make one up.
	Correlation func() string

	// BindProfile binds a run context to a roster profile: the run carries
	// the agent's soul, its model chain, its tool policy and its identity.
	BindProfile func(context.Context, roster.Profile) context.Context

	// CapCost caps the cumulative provider spend of a run (USD-microcents).
	CapCost func(context.Context, int64) context.Context
}

// profile is the roster row named by ref.
//
// The host that holds no roster answers "no such agent" rather than panicking:
// the roster is the agent list, and a fleet that is not there is not an error.
func (ports Ports) profile(ref string) (roster.Profile, bool) {
	if ports.Profile == nil {
		return roster.Profile{}, false
	}
	return ports.Profile(ref)
}

// profiles is the roster.
//
// It is empty for a host that holds no roster.
func (ports Ports) profiles() []roster.Profile {
	if ports.Profiles == nil {
		return nil
	}
	return ports.Profiles()
}

// scan is the reaper's fleet scan.
//
// It answers with an EMPTY report for a host that holds no scanner: no dead
// agents, no degraded ones, nothing to repair.
func (ports Ports) scan(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
	if ports.Scan == nil {
		return api.ReaperReport{}
	}
	return ports.Scan(agentIdleCutoffMs, artifactStaleCutoffMs)
}

// journalRange walks the durable journal once.
//
// A host that holds no journal has no history: the walk reads nothing.
func (ports Ports) journalRange(fn func(*event.Event) error) error {
	if ports.Journal == nil {
		return nil
	}
	return ports.Journal(fn)
}

// liveProvider is the host's live provider.
//
// It is nil for a host that holds no provider, and the module reads that as
// "no live model chain" rather than reaching into nothing.
func (ports Ports) liveProvider() llm.Provider {
	if ports.LiveProvider == nil {
		return nil
	}
	return ports.LiveProvider()
}

// correlation mints the correlation a wake runs under.
//
// It is empty for a host that mints none.
func (ports Ports) correlation() string {
	if ports.Correlation == nil {
		return ""
	}
	return ports.Correlation()
}

// bindProfile binds a run context to a roster profile.
//
// It REFUSES (returns the context unchanged) for a host that holds no door:
// the caller checks the door before it wakes anyone, because a wake without a
// profile would run somebody else.
func (ports Ports) bindProfile(ctx context.Context, p roster.Profile) context.Context {
	if ports.BindProfile == nil {
		return ctx
	}
	return ports.BindProfile(ctx, p)
}

// capCost caps the cumulative provider spend of a run.
//
// It returns the context unchanged for a host that holds no door.
func (ports Ports) capCost(ctx context.Context, mc int64) context.Context {
	if ports.CapCost == nil {
		return ctx
	}
	return ports.CapCost(ctx, mc)
}
