// SPDX-License-Identifier: MIT

package restapi

// Update REST handlers (M860). Token-authed, under /api/v1/update.
// The update service is injected via SetUpdateService so this package stays
// decoupled from kernel/update — and that is now TRUE of the code, not only
// of the comment: the port is the CONTRACT's (W5.4c), so this package names
// no update module. When updateSvc is nil the handlers report "update is
// disabled" rather than dereferencing nil.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/restapiapi"
	"github.com/agezt/agezt/kernel/contract/updateapi"
)

// handleUpdateCheck handles GET /api/v1/update.
// Returns: {current, update: {version,sha256,url,notes}|null, up_to_date}
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if s.updateSvc == nil {
		writeJSON(w, http.StatusOK, updateapi.Check{
			Current:  brand.Version,
			Status:   "update is disabled",
			UpToDate: true,
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
		writeJSON(w, http.StatusOK, updateapi.Check{
			Current:  result.Current,
			UpToDate: true,
		})
		return
	}

	writeJSON(w, http.StatusOK, updateapi.Check{
		Current: result.Current,
		Update: &updateapi.Release{
			Notes:   result.Update.Notes,
			SHA256:  result.Update.SHA256,
			URL:     result.Update.URL,
			Version: result.Update.Version,
		},
	})
}

// handleUpdateApply handles POST /api/v1/update/apply.
// Body: {"version":"1.2.3","sha256":"...","url":"...","notes":"..."}
// Returns: {applied:true, version:"1.2.3"} or {applied:false, error:"..."}
//
// The body is the contract's row (restapiapi.RowUpdateApply) and the door
// measures it against that row before the handler reads it, so a mistyped
// body is refused in the operation registry's words.
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

	// The door: the body is measured against the CONTRACT's row before the
	// handler reads it, and a mistyped body is refused in the operation
	// registry's own words ("operation update_apply input: $.version has
	// wrong type: got number, want string"). The route's own 64 KB cap is
	// applied by the transport, so an oversize body answers the surface's 413.
	var args updateApplyArgs
	if !decodeRequestBody(w, r, restapiapi.EndpointUpdateApply, &args) {
		return
	}

	if args.Version == "" || args.SHA256 == "" || args.URL == "" {
		writeErr(w, http.StatusBadRequest, "missing_fields", "version, sha256, and url are required")
		return
	}

	info := &updateapi.UpdateInfo{
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
	drainFn := func(ctx context.Context, timeout time.Duration) updateapi.DrainResult {
		// Best-effort: just report no active runs. The actual drain happens
		// when the operator calls `agezt update --apply`.
		return updateapi.DrainResult{Timeout: false}
	}

	err := s.updateSvc.Apply(ctx, info, drainFn)
	if err != nil {
		if errors.Is(err, updateapi.ErrDrainTimeout) {
			writeJSON(w, http.StatusConflict, updateapi.Applied{
				Applied: false,
				Error:   "drain timed out: in-flight runs did not complete",
			})
			return
		}
		var mismatch *updateapi.ErrChecksumMismatch
		if errors.As(err, &mismatch) {
			writeErr(w, http.StatusBadRequest, "checksum_mismatch", mismatch.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updateapi.Applied{
		Applied: true,
		Version: args.Version,
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
