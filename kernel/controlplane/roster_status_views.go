// SPDX-License-Identifier: MIT
package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"github.com/agezt/agezt/kernel/roster"
)

func (s *Server) agentStatusViews(profiles []roster.Profile) map[string]map[string]any {
	return approster.NewStatus(s.collectRosterStatusSnapshot, nil).Views(profiles)
}
func (s *Server) collectRosterStatusSnapshot(profiles []roster.Profile, reaperCut, routingCut int64) approster.StatusSnapshot {
	rep := s.k.ReaperScan(reaperCut, reaperCut)
	repairs := s.agentRepairSummaries()
	escalations := s.agentEscalationLoadViews(profiles)
	wakes := s.agentWakeStatusViews(profiles)
	out := approster.StatusSnapshot{
		Degraded: map[string]approster.DegradedStatus{}, Misconfigured: map[string]approster.MisconfigurationStatus{}, Routing: map[string]approster.RoutingStatus{},
		Forced: map[string]approster.ForcedStatus{}, ForcedFailed: map[string]approster.ForcedStatus{}, ForcedExhausted: map[string]approster.ForcedStatus{}, Unstable: map[string]approster.UnstableStatus{}, Dead: map[string]approster.DeadStatus{},
		Repairs: repairs, Escalations: escalations, Wakes: wakes,
	}
	// The journal-derived fields — the live run statuses, the last activity,
	// the last autonomy runbook, the mailbox wakes, the policy denials and
	// the routing/retry pressure — are collected by the application over one
	// journal walk.
	approster.CollectJournalStatus(approster.StatusSources{
		Journal: s.k.Journal().Range,
		Runs:    s.statusRunRows,
	}, profiles, routingCut, &out)
	for _, r := range rep.DegradedAgents {
		out.Degraded[r.Slug] = approster.DegradedStatus{Failures: r.Failures, Threshold: r.Threshold, Window: r.Window, LastFailureMS: r.LastFailureMS}
	}
	for _, r := range rep.MisconfiguredAgents {
		out.Misconfigured[r.Slug] = approster.MisconfigurationStatus{Issues: r.Issues}
	}
	for _, r := range rep.RoutingPressure {
		out.Routing[r.Slug] = approster.RoutingStatus{Count: r.Count}
	}
	for _, r := range rep.RoutingForced {
		out.Forced[r.Slug] = approster.ForcedStatus{Count: r.Count, TaskType: r.TaskType, ForcedChain: r.ForcedChain, ForceGeneration: r.ForceGeneration}
	}
	for _, r := range rep.RoutingForcedFailed {
		out.ForcedFailed[r.Slug] = approster.ForcedStatus{Count: r.Count, TaskType: r.TaskType, ForcedChain: r.ForcedChain, ForceGeneration: r.ForceGeneration}
	}
	for _, r := range rep.RoutingForcedExhausted {
		out.ForcedExhausted[r.Slug] = approster.ForcedStatus{Count: r.Count, TaskType: r.TaskType, ForcedChain: r.ForcedChain, ForceGeneration: r.ForceGeneration}
	}
	for _, r := range rep.RoutingUnstable {
		out.Unstable[r.Slug] = approster.UnstableStatus{Count: r.Count, TaskType: r.TaskType, CurrentChain: r.CurrentChain, PreviousChain: r.PreviousChain}
	}
	for _, r := range rep.DeadAgents {
		out.Dead[r.Slug] = approster.DeadStatus{LastActiveMS: r.LastActiveMS}
	}
	return out
}

// statusRunRows folds the kernel's journal into the run rows the status
// collection needs. The application owns what a row means; the host only
// supplies the rows.
func (s *Server) statusRunRows() ([]approster.RunRow, error) {
	rows, err := s.collectRuns(s.k)
	if err != nil {
		return nil, err
	}
	out := make([]approster.RunRow, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		out = append(out, approster.RunRow{
			CorrelationID: r.CorrelationID, Intent: r.Intent, Model: r.Model, Agent: r.Agent,
			SpentMicrocents: r.SpentMicrocents, StartedUnixMS: r.StartedUnixMS,
			ParentCorrelation: r.ParentCorrelation,
			Completed:         r.Completed, Failed: r.Failed, Abandoned: r.Abandoned,
		})
	}
	return out, nil
}
