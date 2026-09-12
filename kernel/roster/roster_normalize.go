// SPDX-License-Identifier: MIT

// Roster: profile normalizers (normalizeProfile + noise/guardian defaults + small helpers).
// Code extracted from roster_validate.go during the Day-118 god-file split.
// Public API unchanged.
package roster



import (
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/ulid"
)

func normalizeProfile(p *Profile, nowMS int64) {
	p.Slug = strings.TrimSpace(p.Slug)
	p.Name = strings.TrimSpace(p.Name)
	p.Soul = strings.TrimSpace(p.Soul)
	p.TaskType = strings.TrimSpace(p.TaskType)
	p.Model = strings.TrimSpace(p.Model)
	p.MemoryScope = strings.TrimSpace(p.MemoryScope)
	p.Workdir = strings.TrimSpace(p.Workdir)
	p.OwnerAgent = strings.TrimSpace(p.OwnerAgent)
	p.ParentAgent = strings.TrimSpace(p.ParentAgent)
	p.Description = strings.TrimSpace(p.Description)
	p.Instructions = compactStrings(p.Instructions)
	p.ToolAllow = compactUniqueStrings(p.ToolAllow)
	p.ToolDeny = compactUniqueStrings(p.ToolDeny)
	p.TrustCeiling = strings.TrimSpace(strings.ToUpper(p.TrustCeiling))
	normalizeProfilePolicies(p)
	if len(p.ConfigOverrides) > 0 {
		out := make(map[string]string, len(p.ConfigOverrides))
		for key, value := range p.ConfigOverrides {
			key = strings.TrimSpace(strings.ToUpper(key))
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(value)
		}
		if len(out) == 0 {
			p.ConfigOverrides = nil
		} else {
			p.ConfigOverrides = out
		}
	}
	mode := strings.TrimSpace(p.Lifecycle.Mode)
	if mode == "" && p.Lifecycle.RetireOnComplete {
		mode = LifecycleRetireOnComplete
	}
	if (mode == "" || mode == LifecyclePersistent) && p.Lifecycle.MaxCycles > 0 {
		mode = LifecycleCycle
	}
	p.Lifecycle.Mode = mode
	out := make([]AgentTask, 0, len(p.TaskList))
	for _, t := range p.TaskList {
		t.ID = strings.TrimSpace(t.ID)
		t.Title = strings.TrimSpace(t.Title)
		t.Description = strings.TrimSpace(t.Description)
		t.Scope = strings.TrimSpace(t.Scope)
		if t.Scope == "" {
			t.Scope = "total"
		}
		t.Status = strings.TrimSpace(t.Status)
		if t.Status == "" {
			t.Status = "todo"
		}
		if t.Title == "" {
			continue
		}
		if t.ID == "" {
			t.ID = ulid.New()
		}
		if t.CreatedMS == 0 {
			t.CreatedMS = nowMS
		}
		t.UpdatedMS = nowMS
		out = append(out, t)
	}
	p.TaskList = out
	systemDefaultsChanged := applySystemGuardianDefaults(p)
	noiseToolsChanged := enforceNoiseToolDeny(p)
	if systemDefaultsChanged || noiseToolsChanged {
		p.UpdatedMS = nowMS
	}
}

func enforceNoiseToolDeny(p *Profile) bool {
	if p == nil || p.NoisePolicy == nil || !p.NoisePolicy.DisableMemoryWrites {
		return false
	}
	changed := false
	allow := make([]string, 0, len(p.ToolAllow))
	for _, tool := range p.ToolAllow {
		if strings.EqualFold(strings.TrimSpace(tool), "memory") {
			changed = true
			continue
		}
		allow = append(allow, tool)
	}
	deny := make([]string, 0, len(p.ToolDeny)+1)
	hasMemory := false
	for _, tool := range p.ToolDeny {
		if strings.EqualFold(strings.TrimSpace(tool), "memory") {
			if !hasMemory {
				deny = append(deny, "memory")
				hasMemory = true
			}
			if tool != "memory" {
				changed = true
			}
			continue
		}
		deny = append(deny, tool)
	}
	if !hasMemory {
		deny = append(deny, "memory")
		changed = true
	}
	if changed {
		p.ToolAllow = allow
		p.ToolDeny = compactUniqueStrings(deny)
	}
	return changed
}

func applySystemGuardianDefaults(p *Profile) bool {
	if p == nil || !p.System {
		return false
	}
	changed := false
	wantScope := defaultSystemGuardianMemoryScopePrefix + strings.TrimSpace(p.Slug)
	if wantScope != defaultSystemGuardianMemoryScopePrefix {
		scope := strings.TrimSpace(p.MemoryScope)
		if scope == "" || !strings.HasPrefix(scope, defaultSystemGuardianMemoryScopePrefix) {
			p.MemoryScope = wantScope
			changed = true
		}
	}
	if p.MaxCostMc <= 0 {
		p.MaxCostMc = defaultSystemGuardianMaxCostMc
		changed = true
	}
	if p.MaxDailyMc <= 0 {
		p.MaxDailyMc = defaultSystemGuardianMaxDailyMc
		changed = true
	}
	if strings.TrimSpace(p.TrustCeiling) == "" {
		p.TrustCeiling = defaultSystemGuardianTrustCeiling
		changed = true
	}
	if p.NoisePolicy == nil {
		p.NoisePolicy = &NoisePolicy{}
		changed = true
	}
	if !p.NoisePolicy.SilentOnSuccess {
		p.NoisePolicy.SilentOnSuccess = true
		changed = true
	}
	if !p.NoisePolicy.DisableMemoryWrites {
		p.NoisePolicy.DisableMemoryWrites = true
		changed = true
	}
	if noiseSeverityRank(p.NoisePolicy.MinNotifySeverity) < noiseSeverityRank(defaultSystemGuardianMinNotifySeverity) {
		p.NoisePolicy.MinNotifySeverity = defaultSystemGuardianMinNotifySeverity
		changed = true
	}
	if p.NoisePolicy.MinNotifyIntervalSec < defaultSystemGuardianNotifyCooldownSec {
		p.NoisePolicy.MinNotifyIntervalSec = defaultSystemGuardianNotifyCooldownSec
		changed = true
	}
	return changed
}

func noiseSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func normalizeProfilePolicies(p *Profile) {
	if p.RetryPolicy != nil {
		p.RetryPolicy.Backoff = strings.TrimSpace(p.RetryPolicy.Backoff)
		p.RetryPolicy.RetryOn = compactStrings(p.RetryPolicy.RetryOn)
	}
	if p.HealthPolicy != nil {
		p.HealthPolicy.DoctorAgent = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
	}
	if p.SelfRepairPolicy != nil {
		p.SelfRepairPolicy.EscalateTo = strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	if p.NoisePolicy != nil {
		p.NoisePolicy.MinNotifySeverity = strings.ToLower(strings.TrimSpace(p.NoisePolicy.MinNotifySeverity))
	}
}

func compactStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func compactUniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

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

// Store is the persistent roster, a single JSON file rewritten atomically on
// change. Safe for concurrent use. Mirrors kernel/standing.Store.