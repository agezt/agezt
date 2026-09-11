// SPDX-License-Identifier: MIT

package controlplane

// Server lifecycle + runtime plumbing: NewServer constructor, Start/Stop
// shutdown, Addr/Token accessors, token verification, accept loop, runtime
// files writer, etc. Carved out of server.go during the Day 27 god file
// split #3 so the main file is just the Server struct + Set* dependencies.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
	"github.com/agezt/agezt/kernel/runtime"
)

func NewServer(k *runtime.Kernel, baseDir string) *Server {
	return &Server{
		k:          k,
		baseDir:    baseDir,
		shutdownCh: make(chan struct{}),
	}
}

// Shutdown returns a channel that closes when a client has issued
// CmdShutdown. The daemon's main loop should select on it next to
// the OS-signal channel so `agt shutdown` reaches the same orderly
// exit path as Ctrl+C. The channel never re-opens; the daemon must
// treat a close as terminal.
func (s *Server) Shutdown() <-chan struct{} { return s.shutdownCh }

// signalShutdown closes shutdownCh exactly once. Used by
// handleShutdown after the OK response has been written to the
// client, so the client read completes before the daemon starts
// tearing the process down.
func (s *Server) signalShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdownCh) })
}

// Start binds to localhost on an ephemeral port, writes the addr+token
// files, and serves connections until ctx is cancelled or Stop is called.
// Returns once the listener is ready; the accept loop runs in a goroutine.
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

// Addr returns the server's bound TCP address (host:port). Empty before Start.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Token returns the server's auth token. Empty before Start.
func (s *Server) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// tokenIsPrimary reports whether presented equals the primary (admin)
// token, using a constant-time comparison (M187). The primary token is
// the daemon's most privileged credential — it authorizes every command
// on every tenant — so a plain `==`/`!=`, which returns as soon as the
// first differing byte is found, leaks the token byte-by-byte to anyone
// who can time the response. This matches the constant-time check the
// tenant registry already uses (tenant.Registry.Authorize). Length
// differences are revealed (the token is fixed-length hex, so length is
// public anyway), but the secret content is compared in constant time.
func (s *Server) tokenIsPrimary(presented string) bool {
	want := s.Token()
	// A blank presented or server token never authorizes (defense in
	// depth, mirroring tenant.Registry.Authorize): subtle.ConstantTimeCompare
	// of two empty strings returns 1, which would let an empty token match
	// an as-yet-unset server token. Emptiness is not token-content, so this
	// short-circuit leaks nothing secret.
	if want == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(want)) == 1
}

// maxRequestBytes bounds a single control-plane request line (M188). The
// request is read before authentication, so any local client reaching
// the loopback port can stream bytes here; 16 MiB is far above any
// legitimate command (even a large inline run prompt) while bounding a
// pre-auth memory-exhaustion DoS.
const maxRequestBytes = 16 << 20

// errRequestTooLarge is returned when a request line exceeds maxRequestBytes.
var errRequestTooLarge = errors.New("controlplane: request exceeds max size")

// readBoundedLine reads one newline-delimited line from r, bounding the
// total to max bytes (M188). It reads in buffer-sized ReadSlice chunks
// (which return bufio.ErrBufferFull for a line longer than the reader's
// buffer), copying each out before the next read so the returned slice is
// stable, and returns errRequestTooLarge once the accumulated line would
// exceed max — instead of allocating without bound. A trailing chunk with
// io.EOF (stream ended mid-line) is returned with that error.
func readBoundedLine(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(buf)+len(chunk) > max {
			return nil, errRequestTooLarge
		}
		buf = append(buf, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return buf, err
	}
}

// Stop closes the listener and removes the runtime files. Idempotent;
// safe to call from cleanup hooks even when Start was driven by ctx.
func (s *Server) Stop() error {
	err := s.initiateShutdown()
	s.wg.Wait()
	return err
}

// initiateShutdown closes the listener and signals the ctx-watcher goroutine
// to exit. Idempotent.
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
	if err := os.WriteFile(filepath.Join(dir, addrFile), []byte(addr+"\n"), 0o600); err != nil {
		return fmt.Errorf("controlplane: write addr file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, tokenFile), []byte(s.token+"\n"), 0o600); err != nil {
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

// cancelOnConnClose derives a child context that is cancelled when the client
// closes conn. Dispatch applies it to every StreamLive command (research/
// council/conductor/planner/chat-summarize): a disconnected client means the
// result can no longer be delivered, so continuing to spend model calls is
// pure waste. It generalizes the run disconnect sentinel in handleRun: since
// these handlers read exactly one request and the client then sends nothing, a
// blocking Read unblocks only on disconnect (or when the handler itself
// returns and handleConn closes the conn, at which point the cancel is a
// harmless no-op). The caller must defer the returned cancel, and handlers
// must never layer a second call on the same conn — two goroutines reading one
// conn race (pulse_subscribe runs its own watcher and is therefore dispatched
// without this wrapper). Unlike run, this is always on: these handlers have
// no detach path, so there is nothing to preserve by keeping the work alive.
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

