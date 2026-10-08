// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"fmt"
	appupdate "github.com/agezt/agezt/kernel/app/update"
	core "github.com/agezt/agezt/kernel/update"
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

func (s *Server) operatorUpdate() *appupdate.Service {
	return appupdate.New(s.updateSvc, core.CurrentVersion, func(_ context.Context, timeout time.Duration) core.DrainResult {
		timedOut, active := s.k.DrainAndHalt(timeout)
		return core.DrainResult{Timeout: timedOut, ActiveRuns: active}
	}, s.writeUpdateSentinel, func(delay time.Duration) { go func() { time.Sleep(delay); s.signalShutdown() }() })
}
