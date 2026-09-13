// SPDX-License-Identifier: MIT

// Package roster: small normalize helpers (enforceNoiseToolDeny +
// applySystemGuardianDefaults + noiseSeverityRank + compactStrings +
// compactUniqueStrings). Extracted from roster_normalize.go during the
// Day-211 god-file split. Public API unchanged.
package roster


import (
	"strings"
)

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
