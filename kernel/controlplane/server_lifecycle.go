package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane Server public
//             lifecycle (NewServer, Shutdown, Start, Addr, Token, Stop). Extracted
//             from server_lifecycle.go during Day 211 god-file refactor (#88).
//             Public API unchanged.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"

	"github.com/agezt/agezt/kernel/app/binding"
)

// NewServer constructs a Server for one scope.
//
// The scope is what the COMPOSITION ROOT builds: the families' ports, filled
// from the daemon's own kernel. The server keeps the scope and never the
// handle it was filled from — what it was handed is consumed here and
// dropped, so nothing behind the ports stays reachable from the adapter.
//
// A handle is still accepted for the callers that have one, and it is turned
// into a scope on the spot: the same value, resolved once, with the same
// refusal when it is not the daemon's own.
func NewServer(scope any, baseDir string) *Server {
	return &Server{
		scope:      binding.Resolve(scope, baseDir),
		baseDir:    baseDir,
		shutdownCh: make(chan struct{}),
	}
}
func (s *Server) Shutdown() <-chan struct{} { return s.shutdownCh }
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return errors.New("controlplane: already started")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("controlplane: listen: %w", err)
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		ln.Close()
		return fmt.Errorf("controlplane: rand: %w", err)
	}
	s.token = hex.EncodeToString(tokBytes)
	s.listener = ln
	s.done = make(chan struct{})
	// Derive the serving context so both ctx cancellation AND a direct Stop()
	// (which calls serveCancel via initiateShutdown) unblock streaming handlers.
	serveCtx, serveCancel := context.WithCancel(ctx)
	s.serveCancel = serveCancel

	if err := s.writeRuntimeFiles(ln.Addr().String()); err != nil {
		ln.Close()
		s.listener = nil
		return err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(serveCtx)
	}()
	// React to ctx cancellation by initiating shutdown. This goroutine
	// also exits when Stop is called directly (via s.done).
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		select {
		case <-ctx.Done():
		case <-s.done:
			return
		}
		s.initiateShutdown()
	}()
	return nil
}
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
func (s *Server) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}
func (s *Server) Stop() error {
	err := s.initiateShutdown()
	s.wg.Wait()
	return err
}
