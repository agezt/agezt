// SPDX-License-Identifier: MIT

// rollback_helpers.go: 10 supporting helpers split off from rollback.go during
// the Day 211 god-file refactor (#138). Public API unchanged.
package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/atomicfile"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
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
	case rollbackCheckpointKindFile:
		return applyFileSnapshotCheckpoint(cp)
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

func applyFileSnapshotCheckpoint(cp rollbackCheckpoint) (map[string]any, error) {
	before := cp.Before
	if len(before) == 0 {
		return nil, fmt.Errorf("checkpoint %s is missing file snapshot data", cp.ID)
	}
	path := rollbackString(before["abs_path"])
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
	encoded := rollbackString(before["content_b64"])
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode content_b64: %w", err)
	}
	perm := os.FileMode(0o644)
	if n := rollbackIntNumber(before["mode_perm"]); n > 0 {
		perm = os.FileMode(n)
	}
	if err := writeRollbackFile(path, data, perm); err != nil {
		return nil, err
	}
	return map[string]any{"path": path, "restored": "content", "bytes": len(data)}, nil
}

func rollbackCatalogPath() (string, error) {
	base, err := paths.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(rollbackCatalogRelativePath)), nil
}

func loadRollbackCatalog() (rollbackCatalog, error) {
	path, err := rollbackCatalogPath()
	if err != nil {
		return rollbackCatalog{}, err
	}
	return loadRollbackCatalogAt(path)
}

func loadRollbackCatalogAt(path string) (rollbackCatalog, error) {
	cat := rollbackCatalog{Version: rollbackCatalogVersion}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cat, nil
		}
		return cat, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return cat, nil
	}
	if err := json.Unmarshal(raw, &cat); err != nil {
		return cat, err
	}
	if cat.Version == 0 {
		cat.Version = rollbackCatalogVersion
	}
	return cat, nil
}

func writeRollbackCatalogAt(path string, cat rollbackCatalog) error {
	if cat.Version == 0 {
		cat.Version = rollbackCatalogVersion
	}
	body, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, body, 0o600)
}

func findRollbackCheckpoint(cat rollbackCatalog, id string) (int, *rollbackCheckpoint) {
	for i := range cat.Checkpoints {
		if cat.Checkpoints[i].ID == id {
			return i, &cat.Checkpoints[i]
		}
	}
	return -1, nil
}

func writeRollbackFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, perm)
}

func rollbackString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}

func rollbackIntNumber(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	default:
		return 0
	}
}
