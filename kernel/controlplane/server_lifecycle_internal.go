package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane Server internal
//             lifecycle plumbing (signalShutdown, initiateShutdown,
//             writeRuntimeFiles, acceptLoop, cancelOnConnClose). Extracted from
//             server_lifecycle.go during Day 211 god-file refactor (#88). Public API
//             unchanged.

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/agezt/agezt/internal/atomicfile"
)

func (s *Server) signalShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdownCh) })
}
func (s *Server) initiateShutdown() error {
	var firstErr error
	s.stopOnce.Do(func() {
		s.mu.Lock()
		ln := s.listener
		s.listener = nil
		done := s.done
		serveCancel := s.serveCancel
		s.mu.Unlock()

		if done != nil {
			close(done)
		}
		// Release in-flight streaming handlers (run/pulse) blocking on ctx.Done(),
		// so a direct Stop() doesn't have to wait out the per-connection deadline.
		if serveCancel != nil {
			serveCancel()
		}
		if ln != nil {
			if err := ln.Close(); err != nil {
				firstErr = err
			}
		}
		_ = os.Remove(filepath.Join(s.baseDir, "runtime", addrFile))
		_ = os.Remove(filepath.Join(s.baseDir, "runtime", tokenFile))
	})
	return firstErr
}
func (s *Server) writeRuntimeFiles(addr string) error {
	dir := filepath.Join(s.baseDir, "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("controlplane: mkdir runtime: %w", err)
	}
	// The runtime files are the daemon's IDENTITY on disk: the address a client
	// dials and the token it authenticates with. They are written through the
	// platform's atomic writer (internal/atomicfile), so a reader never sees a
	// half-written file — a torn token would be read as a valid-looking token
	// that fails authentication, and a torn address as an address that cannot
	// be dialled. The writer also re-asserts the mode on every write, so a
	// leftover file from an earlier run cannot keep a wider mode.
	if err := atomicfile.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write addr file: %w", err)
	}
	if err := atomicfile.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write token file: %w", err)
	}
	return nil
}
func (s *Server) acceptLoop(ctx context.Context) {
	for {
		s.mu.Lock()
		ln := s.listener
		s.mu.Unlock()
		if ln == nil {
			return
		}
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed → exit cleanly.
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(ctx, conn)
		}()
	}
}
func cancelOnConnClose(ctx context.Context, conn net.Conn) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		_ = conn.SetReadDeadline(time.Time{}) // clear the 10-min handleConn read deadline
		buf := make([]byte, 1)
		_, _ = conn.Read(buf) // blocks until the client disconnects or the conn closes
		cancel()
	}()
	return cctx, cancel
}
