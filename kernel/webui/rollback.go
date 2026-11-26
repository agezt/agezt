// SPDX-License-Identifier: MIT

package webui

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/rollbackapi"

	"github.com/agezt/agezt/kernel/app/files"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/platform/rollbackstore"
)

const (
	rollbackCatalogVersion       = rollbackstore.CatalogVersion
	rollbackCheckpointKindSkill  = rollbackstore.KindSkill
	rollbackCheckpointKindFlow   = rollbackstore.KindFlow
	rollbackCheckpointKindFile   = rollbackstore.KindFile
	rollbackCheckpointKindConfig = rollbackstore.KindConfig
	rollbackCatalogRelativePath  = rollbackstore.RelativePath
	rollbackApplyTimeout         = 5 * time.Second
)

type rollbackCatalog = rollbackstore.Catalog
type rollbackCheckpoint = rollbackstore.Checkpoint

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
	writeJSON(w, http.StatusOK, rollbackapi.List{
		Checkpoints: checkpoints,
		Count:       len(checkpoints),
		RunID:       runID,
	})
}

func (s *Server) handleRollbackApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}
	var body rollbackApplyBody
	ok, err := decodeNativeBody(w, r, opapi.NativeRollbackApply, &body)
	if !ok {
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + err.Error()})
		}
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
		writeJSON(w, http.StatusOK, rollbackapi.Applied{
			Applied:    false,
			Checkpoint: *cp,
			Reason:     "already applied",
		})
		return
	}
	if cp.Kind == rollbackCheckpointKindFile {
		result, err := s.client.Call(r.Context(), opapi.FileRestore, map[string]any{"id": cp.ID})
		if err != nil {
			status := http.StatusBadGateway
			message := err.Error()
			var remote *controlplane.ErrServerError
			if errors.As(err, &remote) {
				message = remote.Msg
				if remote.Code == files.Denied {
					status = http.StatusForbidden
				} else if remote.Code == files.MarkFailed {
					status = http.StatusInternalServerError
				}
			}
			writeJSON(w, status, map[string]any{"error": message})
			return
		}
		writeJSON(w, http.StatusOK, result)
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
	writeJSON(w, http.StatusOK, rollbackapi.Applied{
		Applied:    true,
		Checkpoint: cat.Checkpoints[idx],
		Result:     res,
	})
}
