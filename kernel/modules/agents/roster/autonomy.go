// SPDX-License-Identifier: MIT

package roster

import "strings"

// AutonomyRunbook builds the machine-readable wake contract for a profile: the
// trigger/route/recovery/sleep behavior the agent operates under. It is the one
// canonical builder shared by every wake-evidence emitter so manual operator
// wakes, schedule fires, standing fires (incl. mailbox), and delegated sub-agent
// spawns all carry an identically-shaped `autonomy_runbook` payload through the
// journal, which the control plane folds into agent status as
// `last_autonomy_runbook`. Keep this the single source of truth — do not
// re-derive the contract shape elsewhere.
func AutonomyRunbook(p Profile) map[string]any {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	trigger := "operator_schedule_channel"
	if !p.AllowsDirectCall() {
		trigger = "delegation_only"
	}
	if p.Retired {
		trigger = "blocked_retired"
	} else if !p.Enabled {
		trigger = "blocked_paused"
	}
	route := "self_owned"
	if !p.AllowsDirectCall() {
		if manager != "" {
			route = "leader:" + manager
		} else {
			route = "leader_missing"
		}
	}
	recovery := "manual"
	retryAttempts := 1
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 0 {
		retryAttempts = p.RetryPolicy.MaxAttempts
	}
	if p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled {
		recovery = "self_repair"
	} else if p.HealthPolicy != nil && strings.TrimSpace(p.HealthPolicy.DoctorAgent) != "" {
		recovery = "doctor:" + strings.TrimSpace(p.HealthPolicy.DoctorAgent)
	} else if retryAttempts > 1 {
		recovery = "retry"
	}
	sleep := strings.TrimSpace(p.Lifecycle.Mode)
	if p.Retired {
		sleep = "graveyard"
	} else if !p.Enabled {
		sleep = "paused"
	} else if sleep == "" {
		if p.Lifecycle.RetireOnComplete {
			sleep = LifecycleRetireOnComplete
		} else if p.Lifecycle.MaxCycles > 0 {
			sleep = LifecycleCycle
		} else {
			sleep = LifecyclePersistent
		}
	}
	out := map[string]any{
		"identity_kind":      p.Kind(),
		"trigger_contract":   trigger,
		"route_contract":     route,
		"recovery_contract":  recovery,
		"sleep_contract":     sleep,
		"direct_callable":    p.AllowsDirectCall(),
		"delegation_manager": manager,
		"retry_attempts":     retryAttempts,
	}
	if p.SelfRepairPolicy != nil {
		out["self_repair_enabled"] = p.SelfRepairPolicy.Enabled
		out["self_repair_attempts"] = p.SelfRepairPolicy.MaxAttempts
	}
	if p.HealthPolicy != nil && strings.TrimSpace(p.HealthPolicy.DoctorAgent) != "" {
		out["doctor_agent"] = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
	}
	return out
}
