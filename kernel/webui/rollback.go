// SPDX-License-Identifier: MIT

package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/controlplane"
)

const (
	rollbackCatalogVersion       = 1
	rollbackCheckpointKindSkill  = "skill.status"
	rollbackCheckpointKindFlow   = "workflow.snapshot"
	rollbackCheckpointKindFile   = "file.snapshot"
	rollbackCheckpointKindConfig = "config.setting"
	rollbackCatalogRelativePath  = "rollback/checkpoints.json"
	rollbackApplyTimeout         = 5 * time.Second
)

type rollbackCatalog struct {
	Version     int                  `json:"version"`
	Checkpoints []rollbackCheckpoint `json:"checkpoints"`
}

type rollbackCheckpoint struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Action       string         `json:"action"`
	RunID        string         `json:"run_id,omitempty"`
	SubjectID    string         `json:"subject_id"`
	SubjectName  string         `json:"subject_name,omitempty"`
	BeforeStatus string         `json:"before_status,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Before       map[string]any `json:"before,omitempty"`
	CreatedMS    int64          `json:"created_ms"`
	AppliedMS    int64          `json:"applied_ms,omitempty"`
}

func (s *Server) handleRollbackCheckpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "GET required"})
		return
	}
	cat, err := loadRollbackCatalog()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		runID = strings.TrimSpace(r.URL.Query().Get("run"))
	}
	checkpoints := append([]rollbackCheckpoint(nil), cat.Checkpoints...)
	if checkpoints == nil {
		checkpoints = []rollbackCheckpoint{}
	}
	if runID != "" {
		filtered := checkpoints[:0]
		for _, cp := range checkpoints {
			if cp.RunID == runID {
				filtered = append(filtered, cp)
			}
		}
		checkpoints = filtered
	}
	sort.SliceStable(checkpoints, func(i, j int) bool {
		return checkpoints[i].CreatedMS > checkpoints[j].CreatedMS
	})
	out := map[string]any{"checkpoints": checkpoints, "count": len(checkpoints)}
	if runID != "" {
		out["run_id"] = runID
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRollbackApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, jsonBodyMax))
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + err.Error()})
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "checkpoint id required"})
		return
	}
	path, err := rollbackCatalogPath()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	idx, cp := findRollbackCheckpoint(cat, id)
	if cp == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "checkpoint not found"})
		return
	}
	if cp.AppliedMS > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"checkpoint": *cp, "applied": false, "reason": "already applied"})
		return
	}
	reason := fmt.Sprintf("rollback checkpoint %s", cp.ID)
	if cp.Action != "" {
		reason += " (" + cp.Action + ")"
	}
	res, err := s.applyRollbackCheckpoint(r.Context(), *cp, reason)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	cat.Checkpoints[idx].AppliedMS = time.Now().UnixMilli()
	if err := writeRollbackCatalogAt(path, cat); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "mark applied: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checkpoint": cat.Checkpoints[idx], "applied": true, "result": res})
}

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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if removeErr := os.Remove(path); removeErr == nil {
				if retryErr := os.Rename(tmp, path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agezt-rollback-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			if removeErr := os.Remove(path); removeErr == nil {
				if retryErr := os.Rename(tmpName, path); retryErr == nil {
					return nil
				} else {
					err = retryErr
				}
			}
		}
		return err
	}
	return nil
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
