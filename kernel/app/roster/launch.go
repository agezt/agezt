// SPDX-License-Identifier: MIT

package roster

import (
	"fmt"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// WakeLaunchPorts are the host's capabilities an operator wake needs. The
// application owns the DECISIONS (does the run retry? what does the terminal
// event say?); the host owns the DATA (how a run is executed, how an event
// reaches the bus).
type WakeLaunchPorts struct {
	// Run executes the agent's intent under the given correlation and returns
	// the answer. The retry policy and the cost ceiling are the application's
	// decisions: a nil policy means a plain run, a zero ceiling means no
	// ceiling.
	Run func(corr, intent string, retry *agentapi.RetryPolicy, maxCostMc int64) (string, error)
	// Publish journals the wake's outcome on the bus.
	Publish func(subject, corr string, payload map[string]any)
}

// LaunchOperatorWake runs an operator wake to completion and journals its
// outcome: the retry decision, the cost ceiling and the terminal event — the
// answer or the error with the whole incident lineage — are the
// application's.
func LaunchOperatorWake(ports WakeLaunchPorts, p agentapi.Profile, corr, intent, reason string, lineage IncidentLineage) {
	runbook := agentapi.AutonomyRunbook(p)
	// A profile asks for retries only when it has a policy with more than one
	// attempt; anything else is a plain run.
	var retry *agentapi.RetryPolicy
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		retry = p.RetryPolicy
	}
	// The agent's cost ceiling applies only when the profile has one.
	var ceiling int64
	if p.MaxCostMc > 0 {
		ceiling = p.MaxCostMc
	}
	answer, err := ports.Run(corr, intent, retry, ceiling)
	if err != nil {
		ports.Publish("agent.wake", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              err.Error(),
			"autonomy_runbook":   runbook,
			"incident_id":        lineage.IncidentID,
			"root_incident_id":   lineage.RootIncidentID,
			"parent_incident_id": lineage.ParentIncidentID,
		})
		return
	}
	ports.Publish("agent.wake", corr, map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"reason":             reason,
		"answer":             truncate(answer, 300),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.IncidentID,
		"root_incident_id":   lineage.RootIncidentID,
		"parent_incident_id": lineage.ParentIncidentID,
	})
}

// RepairOutcome is what a governed repair reports back: what it applied, how
// the routing chain moved and the answer it produced.
type RepairOutcome struct {
	Applied                       []string
	RoutingTaskType               string
	RoutingTaskModelChain         []string
	PreviousRoutingTaskModelChain []string
	Answer                        string
}

// RepairLaunchPorts are the host's capabilities an operator repair needs.
type RepairLaunchPorts struct {
	// Repair drives the governed repair for the agent.
	Repair func(slug, reason string) (RepairOutcome, error)
	// Publish journals the repair's outcome on the bus.
	Publish func(subject, corr string, payload map[string]any)
	// OnPanic reports a panic that the firewall caught. It may be nil.
	OnPanic func(slug string, r any)
}

// LaunchOperatorRepair drives the governed repair for an accepted operator
// repair and journals its outcome under corr.
//
// The repair runs a full governed run — provider calls, tools, plugin
// subprocesses — on a bare goroutine, with nothing above it able to recover.
// The panic firewall (WF-001) therefore lives here: a panic is caught, reported
// and journaled as a failed repair instead of taking the daemon down.
func LaunchOperatorRepair(ports RepairLaunchPorts, p agentapi.Profile, corr, reason string, lineage IncidentLineage) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if ports.OnPanic != nil {
			ports.OnPanic(p.Slug, r)
		}
		ports.Publish("agent.repair", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              fmt.Sprintf("repair panicked: %v", r),
			"incident_id":        lineage.IncidentID,
			"root_incident_id":   lineage.RootIncidentID,
			"parent_incident_id": lineage.ParentIncidentID,
		})
	}()
	res, err := ports.Repair(p.Slug, reason)
	if err != nil {
		ports.Publish("agent.repair", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              err.Error(),
			"incident_id":        lineage.IncidentID,
			"root_incident_id":   lineage.RootIncidentID,
			"parent_incident_id": lineage.ParentIncidentID,
		})
		return
	}
	ports.Publish("agent.repair", corr, map[string]any{
		"phase":                             "completed",
		"agent":                             p.Slug,
		"reason":                            reason,
		"applied":                           res.Applied,
		"routing_task_type":                 res.RoutingTaskType,
		"routing_task_model_chain":          res.RoutingTaskModelChain,
		"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
		"answer":                            truncate(res.Answer, 300),
		"incident_id":                       lineage.IncidentID,
		"root_incident_id":                  lineage.RootIncidentID,
		"parent_incident_id":                lineage.ParentIncidentID,
	})
}
