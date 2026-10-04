// SPDX-License-Identifier: MIT

package webui

// Provenance: rollback_helpers.go: 10 supporting helpers split off from rollback.go
//             during the Day 211 god-file refactor (#138). Public API unchanged.

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
	"strings"
)

func (s *Server) applyRollbackCheckpoint(ctx context.Context, cp rollbackCheckpoint, reason string) (map[string]any, error) {
	switch cp.Kind {
	case rollbackCheckpointKindSkill:
		if strings.TrimSpace(cp.SubjectID) == "" || strings.TrimSpace(cp.BeforeStatus) == "" {
			return nil, fmt.Errorf("checkpoint %s is missing skill restore data", cp.ID)
		}
		callCtx, cancel := context.WithTimeout(ctx, rollbackApplyTimeout)
		defer cancel()
		return s.client.Call(callCtx, controlplane.CmdSkillRestore, map[string]any{
			"id": cp.SubjectID, "status": cp.BeforeStatus, "reason": reason,
		})
	case rollbackCheckpointKindFlow:
		if len(cp.Before) == 0 {
			return nil, fmt.Errorf("checkpoint %s is missing workflow snapshot data", cp.ID)
		}
		callCtx, cancel := context.WithTimeout(ctx, rollbackApplyTimeout)
		defer cancel()
		return s.client.Call(callCtx, controlplane.CmdWorkflowRestore, map[string]any{
			"workflow": cp.Before, "reason": reason,
		})
	case rollbackCheckpointKindConfig:
		if rollbackable, ok := cp.Before["rollbackable"].(bool); ok && !rollbackable {
			if why := rollbackString(cp.Before["non_rollbackable_reason"]); why != "" {
				return nil, fmt.Errorf("checkpoint %s is audit-only: %s", cp.ID, why)
			}
			return nil, fmt.Errorf("checkpoint %s is audit-only", cp.ID)
		}
		env := rollbackString(cp.Before["env"])
		if env == "" {
			env = cp.SubjectID
		}
		if env == "" {
			return nil, fmt.Errorf("checkpoint %s is missing config env", cp.ID)
		}
		value := ""
		if set, _ := cp.Before["set"].(bool); set {
			value = rollbackString(cp.Before["value"])
		}
		callCtx, cancel := context.WithTimeout(ctx, rollbackApplyTimeout)
		defer cancel()
		return s.client.Call(callCtx, controlplane.CmdConfigSet, map[string]any{"name": env, "value": value})
	default:
		return nil, fmt.Errorf("checkpoint kind %q is not supported yet", cp.Kind)
	}
}

func rollbackCatalogPath() (string, error)                       { return rollbackstore.DefaultPath() }
func loadRollbackCatalog() (rollbackCatalog, error)              { return rollbackstore.Load() }
func loadRollbackCatalogAt(path string) (rollbackCatalog, error) { return rollbackstore.LoadAt(path) }
func writeRollbackCatalogAt(path string, cat rollbackCatalog) error {
	return rollbackstore.WriteAt(path, cat)
}
func findRollbackCheckpoint(cat rollbackCatalog, id string) (int, *rollbackCheckpoint) {
	return rollbackstore.Find(cat, id)
}
func rollbackString(value any) string { return rollbackstore.StringValue(value) }
