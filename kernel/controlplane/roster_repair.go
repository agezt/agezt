// SPDX-License-Identifier: MIT

package controlplane

// Agent repair handler (M846): the journaled auto-repair entry point.
// Carved out of roster.go during the Day 24 god file split #6 so the main
// file can focus on wake/resolve/escalation.

import (
	"fmt"
	"net"
	"os"
	"strings"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

func (s *Server) handleAgentRepair(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "repaired")})
		return
	}
	corr := s.k.NewCorrelation()
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	lineage := operatorIncidentLineage(req.Args)
	publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go func() {
		// Panic firewall (WF-001). This is the operator's "Repair" button: it
		// answers "accepted" immediately and drives a full governed run — provider
		// calls, tools, plugin subprocesses — on a bare `go`, with nothing above it
		// able to recover. Missed in the original sweep because the goroutine is a
		// closure in a roster handler rather than in one of the runner packages.
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
	}()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

