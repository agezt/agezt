// SPDX-License-Identifier: MIT

package controlplane

// Agent repair run (M846): the governed repair behind the journaled operator
// repair operation.
// Carved out of roster.go during the Day 24 god file split #6 so the main
// file can focus on wake/resolve/escalation.

import (
	"fmt"
	"os"

	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

// runAgentRepair drives the governed repair for an accepted operator repair
// and journals its outcome under corr. Callers start it on its own goroutine.
func (s *Server) runAgentRepair(corr string, p roster.Profile, reason string, lineage operatorWakeLineage) {
	// Panic firewall (WF-001). This is the operator's "Repair" button: it
	// answers "accepted" immediately and drives a full governed run — provider
	// calls, tools, plugin subprocesses — on a bare `go`, with nothing above it
	// able to recover. Missed in the original sweep because the goroutine was a
	// closure in the roster handler rather than in one of the runner packages.
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "operator repair of %q panicked: %v\n", p.Slug, r)
		publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              fmt.Sprintf("repair panicked: %v", r),
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		})
	}()
	src := overseertool.NewKernelSource(s.k, s.baseDir)
	res, err := src.RepairAgent(p.Slug, reason)
	if err != nil {
		publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              err.Error(),
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		})
		return
	}
	publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
		"phase":                             "completed",
		"agent":                             p.Slug,
		"reason":                            reason,
		"applied":                           res.Applied,
		"routing_task_type":                 res.RoutingTaskType,
		"routing_task_model_chain":          res.RoutingTaskModelChain,
		"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
		"answer":                            truncate(res.Answer, 300),
		"incident_id":                       lineage.incidentID,
		"root_incident_id":                  lineage.rootIncidentID,
		"parent_incident_id":                lineage.parentIncidentID,
	})
}
