// SPDX-License-Identifier: MIT

// Package main: per-domain rollback-checkpoint savers + helpers (skill status
// / workflow snapshot / config setting — save + fetch + new each) +
// rollbackCheckpointID + validateSkillStatusCheckpointAction. Extracted from
// rollback_apply.go during the Day-211 god-file split. Public API unchanged.
package main


import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
)

func saveSkillStatusRollbackCheckpoint(ctx context.Context, c *controlplane.Client, action, id, reason string) (*rollbackCheckpoint, error) {
	sk, found, err := workshopFetchSkill(ctx, c, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s not found", id)
	}
	if err := validateSkillStatusCheckpointAction(action, str(sk["status"])); err != nil {
		return nil, err
	}
	cp := newSkillStatusRollbackCheckpoint(action, reason, sk, time.Now())
	if err := appendRollbackCheckpoint(cp); err != nil {
		return nil, err
	}
	return &cp, nil
}

func newSkillStatusRollbackCheckpoint(action, reason string, sk map[string]any, now time.Time) rollbackCheckpoint {
	id := str(sk["id"])
	before := make(map[string]any, len(sk))
	for k, v := range sk {
		before[k] = v
	}
	return rollbackCheckpoint{
		ID:           rollbackCheckpointID(now, id),
		Kind:         rollbackCheckpointKindSkill,
		Action:       action,
		SubjectID:    id,
		SubjectName:  str(sk["name"]),
		BeforeStatus: str(sk["status"]),
		Reason:       reason,
		Before:       before,
		CreatedMS:    now.UnixMilli(),
	}
}

func saveWorkflowSnapshotRollbackCheckpointIfFound(ctx context.Context, c *controlplane.Client, action, ref, reason string) (*rollbackCheckpoint, bool, error) {
	w, found, err := fetchWorkflowSnapshot(ctx, c, ref)
	if err != nil || !found {
		return nil, found, err
	}
	cp := newWorkflowSnapshotRollbackCheckpoint(action, reason, w, time.Now())
	if err := appendRollbackCheckpoint(cp); err != nil {
		return nil, true, err
	}
	return &cp, true, nil
}

func fetchWorkflowSnapshot(ctx context.Context, c *controlplane.Client, ref string) (map[string]any, bool, error) {
	res, err := c.Call(ctx, controlplane.CmdWorkflowShow, map[string]any{"ref": ref})
	if err != nil {
		if strings.Contains(err.Error(), "unknown workflow") {
			return nil, false, nil
		}
		return nil, false, err
	}
	w, _ := res["workflow"].(map[string]any)
	if w == nil {
		return nil, false, nil
	}
	return w, true, nil
}

func newWorkflowSnapshotRollbackCheckpoint(action, reason string, w map[string]any, now time.Time) rollbackCheckpoint {
	id := str(w["id"])
	before := make(map[string]any, len(w))
	for k, v := range w {
		before[k] = v
	}
	return rollbackCheckpoint{
		ID:          rollbackCheckpointID(now, id),
		Kind:        rollbackCheckpointKindFlow,
		Action:      action,
		SubjectID:   id,
		SubjectName: str(w["name"]),
		Reason:      reason,
		Before:      before,
		CreatedMS:   now.UnixMilli(),
	}
}

func saveConfigSettingRollbackCheckpoint(ctx context.Context, c *controlplane.Client, action, env string) (*rollbackCheckpoint, error) {
	fields, err := fetchConfigValueFields(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, raw := range fields {
		m, _ := raw.(map[string]any)
		if m == nil || str(m["env"]) != env {
			continue
		}
		cp := newConfigSettingRollbackCheckpoint(action, m, time.Now())
		if err := appendRollbackCheckpoint(cp); err != nil {
			return nil, err
		}
		return &cp, nil
	}
	return nil, fmt.Errorf("unknown setting %s", env)
}

func fetchConfigValueFields(ctx context.Context, c *controlplane.Client) ([]any, error) {
	res, err := c.Call(ctx, controlplane.CmdConfigValues, nil)
	if err != nil {
		return nil, err
	}
	fields, _ := res["fields"].([]any)
	return fields, nil
}

func newConfigSettingRollbackCheckpoint(action string, field map[string]any, now time.Time) rollbackCheckpoint {
	env := str(field["env"])
	secret, _ := field["secret"].(bool)
	set, _ := field["set"].(bool)
	before := map[string]any{
		"env":          env,
		"secret":       secret,
		"set":          set,
		"env_pinned":   field["env_pinned"],
		"rollbackable": true,
	}
	if secret {
		if set {
			before["rollbackable"] = false
			before["non_rollbackable_reason"] = "previous secret value is masked by the daemon"
		}
	} else {
		before["value"] = str(field["value"])
	}
	return rollbackCheckpoint{
		ID:          rollbackCheckpointID(now, env),
		Kind:        rollbackCheckpointKindConfig,
		Action:      action,
		SubjectID:   env,
		SubjectName: env,
		Before:      before,
		CreatedMS:   now.UnixMilli(),
	}
}

func rollbackCheckpointID(now time.Time, subjectID string) string {
	short := shortID(subjectID)
	if short == "" {
		short = "unknown"
	}
	return fmt.Sprintf("rb-%d-%s", now.UnixNano(), short)
}

func validateSkillStatusCheckpointAction(action, status string) error {
	switch action {
	case "apply":
		if status == "draft" || status == "shadow" || status == "quarantined" {
			return nil
		}
	case "reject":
		if status == "draft" || status == "shadow" {
			return nil
		}
	case "quarantine":
		if status == "active" || status == "shadow" {
			return nil
		}
	case "curate.quarantine":
		if status == "active" {
			return nil
		}
	default:
		return nil
	}
	return fmt.Errorf("%s cannot checkpoint %s skill", action, status)
}
