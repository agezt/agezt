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
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	cat, err := loadRollbackCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body rollbackApplyBody
	ok, err := decodeNativeBody(w, r, opapi.NativeRollbackApply, &body)
	if !ok {
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		}
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "checkpoint id required")
		return
	}
	path, err := rollbackCatalogPath()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cat, err := loadRollbackCatalogAt(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx, cp := findRollbackCheckpoint(cat, id)
	if cp == nil {
		writeError(w, http.StatusNotFound, "checkpoint not found")
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
		result, err := s.client.Call(r.Context(), opapi.FileRestore, opapi.Args(opapi.FileRestoreRequest{ID: cp.ID}))
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
			writeError(w, status, message)
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
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	cat.Checkpoints[idx].AppliedMS = time.Now().UnixMilli()
	if err := writeRollbackCatalogAt(path, cat); err != nil {
		writeError(w, http.StatusInternalServerError, "mark applied: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rollbackapi.Applied{
		Applied:    true,
		Checkpoint: cat.Checkpoints[idx],
		Result:     res,
	})
}
