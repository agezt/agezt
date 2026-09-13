// SPDX-License-Identifier: MIT

// Package roster: profile normalizers (normalizeProfile +
// normalizeProfilePolicies — the top-level normalize dispatcher).
// The small normalize helpers (enforceNoiseToolDeny +
// applySystemGuardianDefaults + noiseSeverityRank + compactStrings +
// compactUniqueStrings) moved to roster_normalize_helpers.go; the per-field
// validators moved to roster_normalize_validate.go. Day-211 god-file split.
// Public API unchanged.
package roster


import (
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
