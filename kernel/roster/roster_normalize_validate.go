// SPDX-License-Identifier: MIT

// Package roster: per-field validators (validateLifecycle + validateTaskList
// + validateRetryPolicy + validateHealthPolicy + validateSelfRepairPolicy +
// validateNoisePolicy). Extracted from roster_normalize.go during the
// Day-211 god-file split. Public API unchanged.
package roster


import (
	"errors"
	"fmt"
	"strings"
)

func validateLifecycle(l AgentLifecycle) error {
	mode := strings.TrimSpace(l.Mode)
	switch mode {
	case "", LifecyclePersistent, LifecycleCycle, LifecycleRetireOnComplete:
	default:
		return errors.New("roster: lifecycle.mode must be persistent, cycle, or retire_on_complete")
	}
	if l.MaxCycles < 0 {
		return errors.New("roster: lifecycle.max_cycles must be >= 0")
	}
	if l.CompletedCycles < 0 {
		return errors.New("roster: lifecycle.completed_cycles must be >= 0")
	}
	return nil
}

func validateTaskList(tasks []AgentTask) error {
	if len(tasks) > 200 {
		return errors.New("roster: at most 200 agent tasks")
	}
	for _, t := range tasks {
		if strings.TrimSpace(t.Title) == "" {
			return errors.New("roster: tasklist title required")
		}
		if len(t.Title) > 256 {
			return errors.New("roster: tasklist title exceeds 256 bytes")
		}
		if len(t.Description) > 4096 {
			return errors.New("roster: tasklist description exceeds 4096 bytes")
		}
		switch strings.TrimSpace(t.Scope) {
		case "", "cycle", "total":
		default:
			return errors.New("roster: tasklist scope must be cycle or total")
		}
		switch strings.TrimSpace(t.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			return errors.New("roster: tasklist status must be todo, doing, done, blocked, or retired")
		}
		if strings.ContainsAny(t.ID, " \t\r\n") || len(t.ID) > 128 {
			return errors.New("roster: tasklist id must be a compact id")
		}
	}
	return nil
}

func validateRetryPolicy(p RetryPolicy) error {
	if p.MaxAttempts < 0 || p.MaxAttempts > 10 {
		return errors.New("roster: retry_policy.max_attempts must be 0..10")
	}
	if p.BaseDelaySec < 0 || p.BaseDelaySec > 3600 {
		return errors.New("roster: retry_policy.base_delay_sec must be 0..3600")
	}
	if p.MaxDelaySec < 0 || p.MaxDelaySec > 86400 {
		return errors.New("roster: retry_policy.max_delay_sec must be 0..86400")
	}
	if p.MaxDelaySec > 0 && p.BaseDelaySec > p.MaxDelaySec {
		return errors.New("roster: retry_policy.base_delay_sec must be <= max_delay_sec")
	}
	backoff := strings.TrimSpace(p.Backoff)
	if backoff != "" && backoff != "fixed" && backoff != "exponential" {
		return errors.New("roster: retry_policy.backoff must be fixed or exponential")
	}
	for _, r := range p.RetryOn {
		switch strings.TrimSpace(r) {
		case "error", "timeout", "canceled", "halted":
		default:
			return errors.New("roster: retry_policy.retry_on values must be error, timeout, canceled, or halted")
		}
	}
	return nil
}

func validateHealthPolicy(p HealthPolicy) error {
	if p.StaleAfterSec < 0 || p.FailureWindow < 0 || p.FailureThreshold < 0 {
		return errors.New("roster: health_policy numeric fields must be >= 0")
	}
	if p.DoctorAgent != "" && !slugRe.MatchString(strings.TrimSpace(p.DoctorAgent)) {
		return fmt.Errorf("roster: health_policy.doctor_agent must match %s", slugRe)
	}
	return nil
}

func validateSelfRepairPolicy(p SelfRepairPolicy) error {
	if p.MaxAttempts < 0 || p.MaxAttempts > 10 {
		return errors.New("roster: self_repair.max_attempts must be 0..10")
	}
	if p.EscalateTo != "" && !slugRe.MatchString(strings.TrimSpace(p.EscalateTo)) {
		return fmt.Errorf("roster: self_repair.escalate_to must match %s", slugRe)
	}
	return nil
}

func validateNoisePolicy(p NoisePolicy) error {
	if p.MinNotifyIntervalSec < 0 || p.MinNotifyIntervalSec > 30*24*3600 {
		return errors.New("roster: noise_policy.min_notify_interval_sec must be 0..2592000")
	}
	switch strings.ToLower(strings.TrimSpace(p.MinNotifySeverity)) {
	case "", "info", "warning", "critical":
	default:
		return errors.New("roster: noise_policy.min_notify_severity must be info, warning, or critical")
	}
	return nil
}
