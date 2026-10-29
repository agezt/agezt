// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"fmt"
	"github.com/agezt/agezt/internal/brand"
	appupdate "github.com/agezt/agezt/kernel/app/update"
	"github.com/agezt/agezt/kernel/contract/updateapi"
	"os"
	"path/filepath"
	"time"
)

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

// operatorUpdate builds the app's update service over the daemon's update
// port.
//
// The rows are the CONTRACT's (W5.4c) and the current version is the
// BRAND's: there is one current version in the tree, and the update module
// used to keep a second copy of it.
func (s *Server) operatorUpdate() *appupdate.Service {
	return appupdate.New(s.updateSvc, brand.Version, func(_ context.Context, timeout time.Duration) updateapi.DrainResult {
		timedOut, active, err := s.lifecyclePorts().Drain(timeout)
		if err != nil {
			// The daemon could not be drained. The update must NOT land on a
			// daemon that is still running, so the flow is told the drain
			// timed out and leaves the binary alone.
			return updateapi.DrainResult{Timeout: true}
		}
		return updateapi.DrainResult{Timeout: timedOut, ActiveRuns: active}
	}, s.writeUpdateSentinel, func(delay time.Duration) { go func() { time.Sleep(delay); s.signalShutdown() }() })
}
