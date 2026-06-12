// SPDX-License-Identifier: MIT

package restapi

// Update REST handlers (M860). Token-authed, under /api/v1/update.
// The update service is injected via SetUpdateService so this package stays
// decoupled from kernel/update. When updateSvc is nil the handlers report
// "update is disabled" rather than dereferencing nil.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/agezt/agezt/kernel/update"
)

// handleUpdateCheck handles GET /api/v1/update.
// Returns: {current, update: {version,sha256,url,notes}|null, up_to_date}
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.updateSvc == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"current":    update.CurrentVersion,
			"up_to_date": true,
			"status":     "update is disabled",
		})
		return
	}

	ctx, cancel := contextWithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := s.updateSvc.Check(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update_check_failed", err.Error())
		return
	}

	if result.Update == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"current":    result.Current,
			"up_to_date": true,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"current":    result.Current,
		"up_to_date": false,
		"update": map[string]any{
			"version": result.Update.Version,
			"sha256":  result.Update.SHA256,
			"url":     result.Update.URL,
			"notes":   result.Update.Notes,
		},
	})
}

// handleUpdateApply handles POST /api/v1/update/apply.
// Body: {"version":"1.2.3","sha256":"...","url":"...","notes":"..."}
// Returns: {applied:true, version:"1.2.3"} or {applied:false, error:"..."}
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if s.updateSvc == nil {
		writeErr(w, http.StatusBadRequest, "update_disabled", "update is disabled")
		return
	}

	if r.ContentLength == 0 {
		writeErr(w, http.StatusBadRequest, "empty_body", "request body required")
		return
	}
	if r.ContentLength > 64*1024 {
		writeErr(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body must be under 64 KB")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read_body", err.Error())
		return
	}

	var args updateApplyArgs
	if err := json.Unmarshal(body, &args); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if args.Version == "" || args.SHA256 == "" || args.URL == "" {
		writeErr(w, http.StatusBadRequest, "missing_fields", "version, sha256, and url are required")
		return
	}

	info := &update.UpdateInfo{
		Version: args.Version,
		SHA256:  args.SHA256,
		URL:     args.URL,
		Notes:   args.Notes,
	}

	// For the REST API, we don't drain (that would kill the HTTP connection).
	// Instead, we validate and stage the update, and the next daemon restart
	// (triggered by the operator or watchdog) will pick it up.
	// The "apply" in REST terms = validate + stage; drain requires a dedicated
	// CLI call that can wait for the connection to close.
	ctx, cancel := contextWithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Use a no-op drain: don't drain via REST (we're mid-HTTP request).
	// The operator triggers the actual drain via `agezt update --apply`.
	drainFn := func(ctx context.Context, timeout time.Duration) update.DrainResult {
		// Best-effort: just report no active runs. The actual drain happens
		// when the operator calls `agezt update --apply`.
		return update.DrainResult{Timeout: false}
	}

	err = s.updateSvc.Apply(ctx, info, drainFn)
	if err != nil {
		if errors.Is(err, update.ErrDrainTimeout) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"applied": false,
				"error":   "drain timed out: in-flight runs did not complete",
			})
			return
		}
		var mismatch *update.ErrChecksumMismatch
		if errors.As(err, &mismatch) {
			writeErr(w, http.StatusBadRequest, "checksum_mismatch", mismatch.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"applied": true,
		"version": args.Version,
	})
}

type updateApplyArgs struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	URL     string `json:"url"`
	Notes   string `json:"notes,omitempty"`
}

// contextWithTimeout is a helper to create a derived context with a timeout,
// safely handling nil parent contexts.
func contextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}
