// SPDX-License-Identifier: MIT

package runtime

// reaper.go is the runtime's face of the agents module's reaper. The scan
// itself — the dead, degraded, misconfigured, retry-pressure, routing-pressure,
// forced-probation and unstable findings, plus the stale-artifact totals —
// lives in kernel/modules/agents/reaper; this file keeps the host's public
// names, hands the module the ports it needs and forwards the scan.
//
// A module may import another module's api package and nothing else
// (archcheck's principle P5), so the runtime never imports the module's
// implementation: it talks to it through api.ReaperScanner and builds its ports
// through api.ReaperPorts. The composition root (the daemon) owns the concrete
// scanner and injects it with SetReaperScanner.

import (
	"strings"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
)

// The rows the host and the module both name are declared once, in the
// module's api package. The runtime keeps its own names as aliases, so every
// importer compiles unchanged while the implementation lives in the module.
type (
	// ReaperAgent is a dead-agent candidate.
	ReaperAgent = api.ReaperAgent
	// DegradedAgent is an agent whose recent runs crossed its health threshold.
	DegradedAgent = api.DegradedAgent
	// MisconfiguredAgent is an agent whose overrides or hierarchy are invalid.
	MisconfiguredAgent = api.MisconfiguredAgent
	// RetryPressureAgent is an agent that keeps hitting transient failures.
	RetryPressureAgent = api.RetryPressureAgent
	// RoutingPressureAgent is an agent that keeps dropping to backup models.
	RoutingPressureAgent = api.RoutingPressureAgent
	// RoutingForcedAgent is an agent whose model chain was forced. The three
	// forced phases are one row; the old names stay as aliases.
	RoutingForcedAgent = api.RoutingForcedAgent
	// RoutingForcedProbationAgent is a forced chain still on probation.
	RoutingForcedProbationAgent = api.RoutingForcedProbationAgent
	// RoutingForcedFailedAgent is a forced chain that failed after it.
	RoutingForcedFailedAgent = api.RoutingForcedFailedAgent
	// RoutingForcedExhaustedAgent is a forced chain exhausted by repetition.
	RoutingForcedExhaustedAgent = api.RoutingForcedExhaustedAgent
	// RoutingUnstableAgent is a chain that rolled back and destabilized again.
	RoutingUnstableAgent = api.RoutingUnstableAgent
	// ReaperReport is the read-only result of a scan.
	ReaperReport = api.ReaperReport
)

// SetReaperScanner injects the agents module's reaper. Called once after Open
// by the composition root (the daemon), which builds the scanner over the
// host's ports with reaper.New(k.ReaperPorts()).
func (k *Kernel) SetReaperScanner(s api.ReaperScanner) {
	k.configMu.Lock()
	k.reaperScanner = s
	k.configMu.Unlock()
}

// reaperService returns the injected reaper, or nil.
func (k *Kernel) reaperService() api.ReaperScanner {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.reaperScanner
}

// ReaperPorts returns the host's ports for the reaper: the journal, the fleet,
// the host's own runtime-config validation, the artifact index, the
// governor's live model chains and the trigger bindings.
//
// It is the runtime's side of the module boundary — the module holds no
// journal, no roster, no artifact index and no provider of its own, so every
// fact it judges comes from the host.
func (k *Kernel) ReaperPorts() api.ReaperPorts {
	return api.ReaperPorts{
		Journal: k.reaperJournalRange,
		Profiles: func() []agentapi.Profile {
			if k.roster == nil {
				return nil
			}
			return k.roster.List()
		},
		RuntimeConfigIssues: func(overrides map[string]string) []api.ConfigIssue {
			return agentRuntimeConfigIssues(overrides)
		},
		StaleArtifacts: func(cutoffMS int64) (int, int64) {
			idx := k.ArtifactIndex()
			if idx == nil {
				return 0, 0
			}
			var count int
			var bytes int64
			for _, e := range idx.StaleEntries(cutoffMS) {
				count++
				bytes += e.Size
			}
			return count, bytes
		},
		TaskModelChain: k.currentTaskModelChain,
		Schedules: func() []api.Binding {
			sched := k.Schedules()
			if sched == nil {
				return nil
			}
			entries := sched.List()
			out := make([]api.Binding, 0, len(entries))
			for _, e := range entries {
				out = append(out, api.Binding{ID: e.ID, Agent: e.Agent})
			}
			return out
		},
		Standing: func() []api.Binding {
			st := k.Standing()
			if st == nil {
				return nil
			}
			orders := st.List()
			out := make([]api.Binding, 0, len(orders))
			for _, o := range orders {
				out = append(out, api.Binding{ID: o.ID, Agent: o.Agent})
			}
			return out
		},
	}
}

// reaperJournalRange walks the durable journal once for the reaper.
func (k *Kernel) reaperJournalRange(fn func(*event.Event) error) error {
	j := k.Journal()
	if j == nil {
		return nil
	}
	return j.Range(fn)
}

// currentTaskModelChain is the governor's live model chain for a task type:
// the chain the host would route that task type over right now.
func (k *Kernel) currentTaskModelChain(taskType string) []string {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return nil
	}
	type taskModelChainsSource interface {
		TaskModelChainsView() map[string][]string
	}
	gov, ok := k.Provider().(taskModelChainsSource)
	if !ok {
		return nil
	}
	src := gov.TaskModelChainsView()[taskType]
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// ReaperScan finds dead-agent candidates and counts stale artifacts. Both
// cutoffs are absolute wall-clock ms — the caller passes `now - grace`.
// Read-only.
//
// A kernel whose composition root did not wire the reaper answers with an
// empty report: the module is injected, not built here (the runtime may not
// import a module's implementation), so an unwired kernel has no scanner to
// ask.
func (k *Kernel) ReaperScan(agentIdleCutoffMs, artifactStaleCutoffMs int64) ReaperReport {
	svc := k.reaperService()
	if svc == nil {
		return ReaperReport{}
	}
	return svc.Scan(agentIdleCutoffMs, artifactStaleCutoffMs)
}
