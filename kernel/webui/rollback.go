// SPDX-License-Identifier: MIT

package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
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
	dec := json.NewDecoder(r.Body)
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
