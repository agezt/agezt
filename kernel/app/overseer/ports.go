// SPDX-License-Identifier: MIT

package overseer

import (
	"context"
	"errors"

	appcorrelation "github.com/agezt/agezt/kernel/app/correlation"
	appprovider "github.com/agezt/agezt/kernel/app/provider"
	appreaper "github.com/agezt/agezt/kernel/app/reaper"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/runtime"
)

// The overseer's ports, filled from the kernel handle.
//
// The overseer supervises the LIVE system: it lists the fleet, reads what a
// change would affect, pauses and retires agents, cancels a run, wakes an
// agent, repairs one, and halts or resumes the daemon itself.
//
// The fleet's verbs are the ROSTER's business, and the daemon's own gate — the
// halt and the resume — is the DAEMON's. Four more capabilities are other
// families' as a WHOLE: the correlation the wake runs under, the run the wake
// starts, the fleet scan the repair reads, and the task model chains the
// routing edits. Those are asked of the families that own them, so a verb is
// never named twice.
//
// Two verbs belong to NOBODY at all: CANCEL RUN and the RUN IDS. The overseer
// is the only family that reads them, so the overseer OWNS them, and the
// guard says so by name (the rule is by whole name, and it is held in both
// directions).
//
// The port holds FUNCTIONS: a question is asked when it is asked, and a daemon
// that grows a capability does not grow the port.
//
// A host with no daemon holds no daemon to supervise. The READS answer with
// nothing — no fleet, no impact, no runs — and the WRITES refuse: a service
// that edited nothing while answering "edited" would be lying about a daemon
// that does not exist.

// ErrNoDaemon is the answer a host with no daemon gives to every WRITE.
//
// The refusal is written down **once**, here, and the source does not spell
// it again.
var ErrNoDaemon = errors.New("overseer: no daemon on this host")

// Ports are the overseer's capabilities.
//
// Every capability is held UNEXPORTED: the daemon may be absent, and the port
// answers for that case instead of handing the caller a nil object to call
// into.
type Ports struct {
	stopped       func() bool
	runningIDs    func() []string
	profiles      func() []agentapi.Profile
	profile       func(string) (agentapi.Profile, bool)
	impact        func(string) []string
	stopRun       func(string) bool
	haltDaemon    func(string)
	resumeDaemon  func(string)
	enableAgent   func(string, bool) (agentapi.Profile, error)
	retireAgent   func(string, bool, string) (agentapi.Profile, error)
	editProfile   func(string, func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	createProfile func(agentapi.Profile) (agentapi.Profile, error)
	deleteProfile func(string) (bool, error)
	correlation   func() string
	launch        func(context.Context, string, string, *agentapi.RetryPolicy) (string, error)
	fleetScan     func(int64) agentapi.ReaperReport
	liveProvider  func() llm.Provider
}

// PortsFromKernel fills the overseer's ports from the kernel handle.
//
// The handle may be nil: a host that holds no daemon holds no daemon to
// supervise, and the ports answer for that case.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	correlation := appcorrelation.PortsFromKernel(k)
	provider := appprovider.PortsFromKernel(k)
	reaper := appreaper.PortsFromKernel(k)
	runs := appruns.PortsFromKernel(k)
	return Ports{
		stopped:      k.IsHalted,
		runningIDs:   k.ActiveRunIDs,
		profiles:     k.RosterList,
		profile:      k.RosterGet,
		impact:       k.AgentImpact,
		stopRun:      k.CancelRun,
		haltDaemon:   func(reason string) { k.HaltWith(reason) },
		resumeDaemon: func(reason string) { k.ResumeWith(reason) },
		enableAgent:  k.SetProfileEnabled,
		retireAgent: func(ref string, retired bool, reason string) (agentapi.Profile, error) {
			if reason == "" {
				return k.SetProfileRetired(ref, retired)
			}
			return k.SetProfileRetired(ref, retired, reason)
		},
		editProfile:   k.UpdateProfile,
		createProfile: k.AddProfile,
		deleteProfile: k.RemoveProfile,
		correlation:   correlation.Mint,
		launch: func(ctx context.Context, corr, intent string, retry *agentapi.RetryPolicy) (string, error) {
			return runs.Launch(ctx, corr, intent, retry)
		},
		fleetScan: func(cutoffMS int64) agentapi.ReaperReport {
			return reaper.Scan(cutoffMS, cutoffMS)
		},
		liveProvider: provider.Live,
	}
}

// Ready reports whether the ports hold a daemon to supervise.
func (p Ports) Ready() bool { return p.profiles != nil }

// Stopped reports whether the daemon has halted.
//
// The name is the daemon overview's: the kernel's own verb for it is
// "IsHalted", and a port that answers to a kernel verb's name is a verb the
// adapter may name.
func (p Ports) Stopped() bool {
	if p.stopped == nil {
		return false
	}
	return p.stopped()
}

// RunningIDs lists the runs in flight.
//
// The name is the overseer's own: the kernel's own verb for it is
// "ActiveRunIDs", and the port may not carry the name of the verb it stands
// for.
func (p Ports) RunningIDs() []string {
	if p.runningIDs == nil {
		return nil
	}
	return p.runningIDs()
}

// Profiles lists the daemon's agents.
func (p Ports) Profiles() []agentapi.Profile {
	if p.profiles == nil {
		return nil
	}
	return p.profiles()
}

// Profile is the agent named by ref.
//
// The second answer says whether the daemon holds such an agent.
func (p Ports) Profile(ref string) (agentapi.Profile, bool) {
	if p.profile == nil {
		return agentapi.Profile{}, false
	}
	return p.profile(ref)
}

// ImpactOf reports what a change to the agent named by slug would affect.
//
// It is empty for a daemon that holds no fleet.
func (p Ports) ImpactOf(slug string) []string {
	if p.impact == nil {
		return nil
	}
	return p.impact(slug)
}

// StopRun cancels the run named by corr.
//
// It is the overseer's own verb: the kernel's own is "CancelRun", and the
// guard says that this one belongs to the overseer.
func (p Ports) StopRun(corr string) bool {
	if p.stopRun == nil {
		return false
	}
	return p.stopRun(corr)
}

// HaltDaemon stops the daemon from starting new runs.
//
// It is the daemon's own gate, asked through the overseer's port.
func (p Ports) HaltDaemon(reason string) {
	if p.haltDaemon == nil {
		return
	}
	p.haltDaemon(reason)
}

// ResumeDaemon lets the daemon start new runs again.
func (p Ports) ResumeDaemon(reason string) {
	if p.resumeDaemon == nil {
		return
	}
	p.resumeDaemon(reason)
}

// EnableAgent pauses or resumes the agent named by ref.
//
// It REFUSES when there is no daemon: a write that answered "done" about a
// daemon that does not exist would be a lie.
func (p Ports) EnableAgent(ref string, enabled bool) (agentapi.Profile, error) {
	if p.enableAgent == nil {
		return agentapi.Profile{}, ErrNoDaemon
	}
	return p.enableAgent(ref, enabled)
}

// RetireAgent retires or restores the agent named by ref.
//
// The reason is optional: an empty reason retires without one.
func (p Ports) RetireAgent(ref string, retired bool, reason string) (agentapi.Profile, error) {
	if p.retireAgent == nil {
		return agentapi.Profile{}, ErrNoDaemon
	}
	return p.retireAgent(ref, retired, reason)
}

// EditProfile edits the agent named by ref.
//
// It REFUSES when there is no daemon.
func (p Ports) EditProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	if p.editProfile == nil {
		return agentapi.Profile{}, false, ErrNoDaemon
	}
	return p.editProfile(ref, mutate)
}

// CreateProfile adds an agent.
//
// It REFUSES when there is no daemon.
func (p Ports) CreateProfile(profile agentapi.Profile) (agentapi.Profile, error) {
	if p.createProfile == nil {
		return agentapi.Profile{}, ErrNoDaemon
	}
	return p.createProfile(profile)
}

// DeleteProfile removes the agent named by ref.
//
// It REFUSES when there is no daemon.
func (p Ports) DeleteProfile(ref string) (bool, error) {
	if p.deleteProfile == nil {
		return false, ErrNoDaemon
	}
	return p.deleteProfile(ref)
}

// NewCorrelationID mints the correlation a wake runs under.
//
// It is empty for a host with no daemon: there is nothing to correlate.
func (p Ports) NewCorrelationID() string {
	if p.correlation == nil {
		return ""
	}
	return p.correlation()
}

// Launch starts one run under the correlation and returns its answer.
//
// The choice between a plain run and a retrying one is the CALLER's: a nil
// policy means a plain run.
func (p Ports) Launch(ctx context.Context, corr, intent string, retry *agentapi.RetryPolicy) (string, error) {
	if p.launch == nil {
		return "", ErrNoDaemon
	}
	return p.launch(ctx, corr, intent, retry)
}

// FleetScan is the reaper's read-only scan over the fleet.
//
// It MAY answer with nothing: the reaper is a MODULE the composition root
// injects, and a daemon that was given none holds no scanner.
func (p Ports) FleetScan(idleCutoffMS int64) agentapi.ReaperReport {
	if p.fleetScan == nil {
		return agentapi.ReaperReport{}
	}
	return p.fleetScan(idleCutoffMS)
}

// LiveProvider is the live provider the daemon routes through.
//
// The provider is the PROVIDER family's business, and the port carries the
// provider itself: the narrow views over it belong to the CONSUMERS, and the
// overseer's own narrow view -- the task model chains -- is declared here, in
// the overseer, and not in the provider's port.
//
// The name is the provider family's: the kernel's own verb for it is
// "Provider", and a port that answers to a kernel verb's name is a verb the
// adapter may name.
func (p Ports) LiveProvider() llm.Provider {
	if p.liveProvider == nil {
		return nil
	}
	return p.liveProvider()
}
