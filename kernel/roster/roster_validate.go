// SPDX-License-Identifier: MIT

// Roster: profile Validate + per-policy sub-validators (Lifecycle/TaskList/Retry/Health/SelfRepair/Noise).
// Code extracted from roster_validate.go during the Day-118 god-file split.
// Public API unchanged.
package roster



import (
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/edict"
	"path/filepath"
)



func Validate(p Profile) error {
	if !slugRe.MatchString(p.Slug) {
		return fmt.Errorf("roster: slug must match %s", slugRe)
	}
	if len(p.Soul) > maxSoulBytes {
		return fmt.Errorf("roster: soul exceeds %d bytes", maxSoulBytes)
	}
	if len(p.Instructions) > 64 {
		return errors.New("roster: at most 64 instructions")
	}
	for _, ins := range p.Instructions {
		if len(ins) > 4096 {
			return errors.New("roster: instruction exceeds 4096 bytes")
		}
	}
	if len(p.Fallbacks) > maxFallbacks {
		return fmt.Errorf("roster: at most %d fallback models", maxFallbacks)
	}
	if len(p.ToolAllow) > 256 {
		return errors.New("roster: at most 256 tool_allow entries")
	}
	if len(p.ToolDeny) > 256 {
		return errors.New("roster: at most 256 tool_deny entries")
	}
	if len(p.ConfigOverrides) > maxConfigOverrides {
		return fmt.Errorf("roster: at most %d config_overrides entries", maxConfigOverrides)
	}
	for _, f := range p.Fallbacks {
		if strings.TrimSpace(f) == "" {
			return errors.New("roster: empty fallback model id")
		}
	}
	if p.MaxCostMc < 0 {
		return errors.New("roster: max_cost_mc must be >= 0")
	}
	if p.MaxDailyMc < 0 {
		return errors.New("roster: max_daily_mc must be >= 0")
	}
	for label, names := range map[string][]string{"tool_allow": p.ToolAllow, "tool_deny": p.ToolDeny} {
		for _, name := range names {
			if !toolNameRe.MatchString(strings.TrimSpace(name)) {
				return fmt.Errorf("roster: %s contains invalid tool name %q", label, name)
			}
		}
	}
	allow := map[string]bool{}
	for _, name := range p.ToolAllow {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			allow[name] = true
		}
	}
	for _, name := range p.ToolDeny {
		if trimmed := strings.ToLower(strings.TrimSpace(name)); trimmed != "" && allow[trimmed] {
			return fmt.Errorf("roster: tool %q cannot be both allowed and denied", strings.TrimSpace(name))
		}
	}
	if strings.TrimSpace(p.TrustCeiling) != "" {
		if _, err := edict.ParseTrustLevel(p.TrustCeiling); err != nil {
			return fmt.Errorf("roster: trust_ceiling: %w", err)
		}
	}
	for key, value := range p.ConfigOverrides {
		if !configKeyRe.MatchString(strings.TrimSpace(key)) {
			return fmt.Errorf("roster: config_overrides key %q must match %s", key, configKeyRe)
		}
		if len(value) > 8192 {
			return fmt.Errorf("roster: config_overrides[%s] exceeds 8192 bytes", key)
		}
	}
	if p.Workdir != "" {
		w := filepath.ToSlash(p.Workdir)
		if filepath.IsAbs(p.Workdir) || strings.HasPrefix(w, "/") ||
			w == ".." || strings.HasPrefix(w, "../") || strings.Contains(w, "/../") || strings.HasSuffix(w, "/..") {
			return errors.New("roster: workdir must be a relative path inside the workspace")
		}
	}
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref != "" && !slugRe.MatchString(ref) {
			return fmt.Errorf("roster: %s must match %s", label, slugRe)
		}
		if ref != "" && ref == strings.TrimSpace(p.Slug) {
			return fmt.Errorf("roster: %s cannot point to the same agent", label)
		}
	}
	if !p.AllowsDirectCall() && strings.TrimSpace(p.OwnerAgent) == "" && strings.TrimSpace(p.ParentAgent) == "" {
		return errors.New("roster: managed sub-agents require owner_agent or parent_agent")
	}
	if p.RetryPolicy != nil {
		if err := validateRetryPolicy(*p.RetryPolicy); err != nil {
			return err
		}
	}
	if p.HealthPolicy != nil {
		if err := validateHealthPolicy(*p.HealthPolicy); err != nil {
			return err
		}
	}
	if p.SelfRepairPolicy != nil {
		if err := validateSelfRepairPolicy(*p.SelfRepairPolicy); err != nil {
			return err
		}
	}
	if p.NoisePolicy != nil {
		if err := validateNoisePolicy(*p.NoisePolicy); err != nil {
			return err
		}
	}
	if err := validateLifecycle(p.Lifecycle); err != nil {
		return err
	}
	if err := validateTaskList(p.TaskList); err != nil {
		return err
	}
	return nil
}

