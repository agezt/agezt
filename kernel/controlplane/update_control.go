// SPDX-License-Identifier: MIT

package controlplane

// Self-update handlers (M860). The update service is injected via
// SetUpdateService so this package stays decoupled from kernel/update.
// When updateSvc is nil the handlers report "update is disabled" rather
// than panicking.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/update"
)

// writeUpdateSentinel writes the update sentinel file to signal the watchdog
// that the daemon is intentionally exiting after a successful self-update.
// This clears the crash-loop backoff on the next spawn. The sentinel is written
// to <baseDir>/update.sentinel.
func (s *Server) writeUpdateSentinel() {
	if s.baseDir == "" {
		return
	}
	p := filepath.Join(s.baseDir, "update.sentinel")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		// Best-effort: sentinel failure should not block the update.
		return
	}
	fmt.Fprintf(f, "%s\n", time.Now().UTC().Format(time.RFC3339))
	f.Close()
}

// handleUpdateCheck queries the update source and reports whether a new version
// is available. No args required.
//
// Returns: {current, update: {version, sha256, url, notes}|null, up_to_date}
func (s *Server) handleUpdateCheck(conn net.Conn, req Request) {
	if s.updateSvc == nil {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespResult,
			Result: map[string]any{
				"current":     update.CurrentVersion,
				"update":      nil,
				"up_to_date":  true,
				"status":      "update is disabled",
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := s.updateSvc.Check(ctx)
	if err != nil {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespError,
			Error: fmt.Sprintf("update check failed: %v", err),
		})
		return
	}

	if result.Update == nil {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespResult,
			Result: map[string]any{
				"current":    result.Current,
				"update":     nil,
				"up_to_date": true,
			},
		})
		return
	}

	s.writeResp(conn, Response{
		ID:    req.ID,
		Type:  RespResult,
		Result: map[string]any{
			"current":    result.Current,
			"up_to_date": false,
			"update": map[string]any{
				"version": result.Update.Version,
				"sha256":  result.Update.SHA256,
				"url":     result.Update.URL,
				"notes":   result.Update.Notes,
			},
		},
	})
}

// handleUpdateApply runs the drain → atomic swap → restart sequence.
// Args required: version, sha256, url. Optional: notes.
//
// Returns: {applied: bool, error?: string}
// On success the daemon will exit 0 and the watchdog spawns the new binary.
// On failure the daemon stays running — human must investigate.
func (s *Server) handleUpdateApply(conn net.Conn, req Request) {
	if s.updateSvc == nil {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespError,
			Error: "update is disabled",
		})
		return
	}

	// Parse required args.
	version, _ := req.Args["version"].(string)
	sha256, _ := req.Args["sha256"].(string)
	url, _ := req.Args["url"].(string)
	notes, _ := req.Args["notes"].(string)

	var errs []string
	if version == "" {
		errs = append(errs, "version is required")
	}
	if sha256 == "" {
		errs = append(errs, "sha256 is required")
	}
	if url == "" {
		errs = append(errs, "url is required")
	}
	if len(errs) > 0 {
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespError,
			Error: strings.Join(errs, "; "),
		})
		return
	}

	info := &update.UpdateInfo{
		Version: version,
		SHA256:  sha256,
		URL:     url,
		Notes:   notes,
	}

	// drainFn delegates to the kernel's DrainAndHalt, which atomically:
	// 1. Cancels all in-flight runs (same as Halt)
	// 2. Waits for them to unwind within the given timeout
	// 3. Returns whether the drain timed out and how many runs were active
	drainFn := func(ctx context.Context, timeout time.Duration) update.DrainResult {
		timedOut, activeRuns := s.k.DrainAndHalt(timeout)
		return update.DrainResult{
			Timeout:    timedOut,
			ActiveRuns: activeRuns,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.updateSvc.Apply(ctx, info, drainFn)
	if err != nil {
		// Distinguish drain timeout (user-visible, no swap occurred).
		if errors.Is(err, update.ErrDrainTimeout) {
			s.writeResp(conn, Response{
				ID:    req.ID,
				Type:  RespResult,
				Result: map[string]any{
					"applied": false,
					"error":   "drain timed out: in-flight runs did not complete within the configured timeout",
				},
			})
			return
		}

		// Checksum mismatch or download failure — human must investigate.
		s.writeResp(conn, Response{
			ID:    req.ID,
			Type:  RespResult,
			Result: map[string]any{
				"applied": false,
				"error":   fmt.Sprintf("update failed: %v", err),
			},
		})
		return
	}

	// Apply succeeded. Write the update sentinel (M860) so the watchdog knows
	// this was an intentional restart (not a crash). The sentinel is checked
	// before the next spawn and clears the crash-loop backoff.
	s.writeUpdateSentinel()

	// Exit cleanly so the watchdog (which started us) can spawn the new binary.
	// The watchdog checks the sentinel and resumes without backoff delay.
	s.writeResp(conn, Response{
		ID:    req.ID,
		Type:  RespResult,
		Result: map[string]any{
			"applied": true,
			"version": version,
		},
	})

	// Exit cleanly so the watchdog (which started us) can spawn the new binary.
	// If the watchdog doesn't restart us, the operator investigates.
	// Use graceful shutdown so journal/state are flushed.
	go func() {
		time.Sleep(100 * time.Millisecond) // let the response flush
		s.signalShutdown()
	}()
}
