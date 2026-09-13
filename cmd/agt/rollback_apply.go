// SPDX-License-Identifier: MIT

// Package main: `agt rollback apply` CLI dispatcher (cmdRollbackApply) +
// applyRollbackCheckpoint + applyFileSnapshotCheckpoint (the apply paths).
// The per-domain checkpoint savers (skill status / workflow snapshot / config
// setting) + their helpers moved to rollback_apply_checkpoints.go.
// Day-211 god-file split. Public API unchanged.
package main


import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)
func cmdRollbackApply(args []string, stdout, stderr io.Writer) int {
	if rollbackHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s rollback apply <checkpoint> [--json]\n", brand.CLI)
		return 0
	}
	id, asJSON, ok := parseRollbackIDJSON(args, "apply", stdout, stderr)
	if !ok {
		return 2
	}
	path, err := rollbackCatalogPath()
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: %v\n", brand.CLI, err)
		return 1
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: %v\n", brand.CLI, err)
		return 1
	}
	idx, cp := findRollbackCheckpoint(cat, id)
	if cp == nil {
		fmt.Fprintf(stderr, "%s rollback apply: checkpoint %s not found\n", brand.CLI, id)
		return 3
	}
	if cp.AppliedMS > 0 {
		out := map[string]any{"checkpoint": *cp, "applied": false, "reason": "already applied"}
		if asJSON {
			return jsonout.Write(stdout, out)
		}
		fmt.Fprintf(stdout, "rollback %s already applied at %s\n", cp.ID, time.UnixMilli(cp.AppliedMS).Format(rollbackDefaultRenderTimeFmt))
		return 0
	}
	reason := fmt.Sprintf("rollback checkpoint %s", cp.ID)
	if cp.Action != "" {
		reason += " (" + cp.Action + ")"
	}
	res, err := applyRollbackCheckpoint(*cp, reason, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: %v\n", brand.CLI, err)
		return 1
	}
	cat.Checkpoints[idx].AppliedMS = time.Now().UnixMilli()
	if err := writeRollbackCatalogAt(path, cat); err != nil {
		fmt.Fprintf(stderr, "%s rollback apply: mark applied: %v\n", brand.CLI, err)
		return 1
	}
	cp = &cat.Checkpoints[idx]
	out := map[string]any{"checkpoint": *cp, "applied": true, "result": res}
	if asJSON {
		return jsonout.Write(stdout, out)
	}
	fmt.Fprintf(stdout, "rolled back %s: %s\n", cp.ID, rollbackApplySummary(*cp))
	return 0
}

func applyRollbackCheckpoint(cp rollbackCheckpoint, reason string, stderr io.Writer) (map[string]any, error) {
	switch cp.Kind {
	case rollbackCheckpointKindSkill:
		if strings.TrimSpace(cp.SubjectID) == "" || strings.TrimSpace(cp.BeforeStatus) == "" {
			return nil, fmt.Errorf("checkpoint %s is missing skill restore data", cp.ID)
		}
		c := dialpkg.New(stderr)
		if c == nil {
			return nil, errors.New("daemon unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), rollbackDefaultApplyTimeout)
		defer cancel()
		return c.Call(ctx, controlplane.CmdSkillRestore, map[string]any{
			"id": cp.SubjectID, "status": cp.BeforeStatus, "reason": reason,
		})
	case rollbackCheckpointKindFlow:
		if len(cp.Before) == 0 {
			return nil, fmt.Errorf("checkpoint %s is missing workflow snapshot data", cp.ID)
		}
		c := dialpkg.New(stderr)
		if c == nil {
			return nil, errors.New("daemon unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), rollbackDefaultApplyTimeout)
		defer cancel()
		return c.Call(ctx, controlplane.CmdWorkflowRestore, map[string]any{
			"workflow": cp.Before, "reason": reason,
		})
	case rollbackCheckpointKindFile:
		return applyFileSnapshotCheckpoint(cp)
	case rollbackCheckpointKindConfig:
		if rollbackable, ok := cp.Before["rollbackable"].(bool); ok && !rollbackable {
			if why := str(cp.Before["non_rollbackable_reason"]); why != "" {
				return nil, fmt.Errorf("checkpoint %s is audit-only: %s", cp.ID, why)
			}
			return nil, fmt.Errorf("checkpoint %s is audit-only", cp.ID)
		}
		env := str(cp.Before["env"])
		if env == "" {
			env = cp.SubjectID
		}
		if env == "" {
			return nil, fmt.Errorf("checkpoint %s is missing config env", cp.ID)
		}
		value := ""
		if set, _ := cp.Before["set"].(bool); set {
			value = str(cp.Before["value"])
		}
		c := dialpkg.New(stderr)
		if c == nil {
			return nil, errors.New("daemon unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), rollbackDefaultApplyTimeout)
		defer cancel()
		return c.Call(ctx, controlplane.CmdConfigSet, map[string]any{"name": env, "value": value})
	default:
		return nil, fmt.Errorf("checkpoint kind %q is not supported yet", cp.Kind)
	}
}

func applyFileSnapshotCheckpoint(cp rollbackCheckpoint) (map[string]any, error) {
	before := cp.Before
	if len(before) == 0 {
		return nil, fmt.Errorf("checkpoint %s is missing file snapshot data", cp.ID)
	}
	path := str(before["abs_path"])
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("checkpoint %s is missing abs_path", cp.ID)
	}
	exists, _ := before["exists"].(bool)
	if li, err := os.Lstat(path); err == nil {
		if li.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to restore through symlink at %s", path)
		}
		if li.IsDir() {
			return nil, fmt.Errorf("refusing to restore over directory at %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return map[string]any{"path": path, "restored": "absent"}, nil
	}
	encoded := str(before["content_b64"])
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode content_b64: %w", err)
	}
	perm := os.FileMode(0o644)
	if n := intNumber(before["mode_perm"]); n > 0 {
		perm = os.FileMode(n)
	}
	if err := writeRollbackFile(path, data, perm); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "restored": "content", "bytes": len(data)}, nil
}
