// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"fmt"
	appupdate "github.com/agezt/agezt/kernel/app/update"
	core "github.com/agezt/agezt/kernel/update"
	"net"
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
func (s *Server) updateReply(conn net.Conn, req Request) func(map[string]any, error) {
	return func(out map[string]any, err error) {
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
	}
}
func (s *Server) handleUpdateCheck(conn net.Conn, req Request) {
	s.operatorUpdate().Check(context.Background(), s.updateReply(conn, req))
}
func (s *Server) handleUpdateApply(conn net.Conn, req Request) {
	args, err := argStrings(req.Args, "version", "sha256", "url", "notes")
	s.operatorUpdate().Apply(context.Background(), appupdate.ApplyInput{Version: args["version"], SHA256: args["sha256"], URL: args["url"], Notes: args["notes"], DecodeError: err}, s.updateReply(conn, req))
}
