// SPDX-License-Identifier: MIT

package main

// stdioTransport carries MCP frames over a child process's stdio
// (M1.MCP-SSE — original transport, factored out of mcp.go).
// Per-line JSON-RPC: one request per line written to stdin, one
// response per line read from stdout. The child's stderr is
// forwarded to our stderr so the agezt host's plugin logger
// picks it up.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type stdioTransport struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	deliver transportDeliver

	mu     sync.Mutex // serializes writes to stdin
	closed bool
}

// newStdioTransport spawns the configured server, wires its
// stdin/stdout into a stdioTransport, and starts the read loop.
// Returns an error if the child fails to start; on success the
// caller owns the transport and must close() it.
func newStdioTransport(path string, args []string, deliver transportDeliver) (*stdioTransport, error) {
	cmd := exec.Command(path, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdio mcp: stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("stdio mcp: start %q: %w", path, err)
	}
	t := &stdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		deliver: deliver,
	}
	go t.readLoop()
	return t, nil
}

func (t *stdioTransport) send(req jsonrpcReq) error {
	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("stdio mcp: marshal request: %w", err)
	}
	raw = append(raw, '\n')
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.stdin.Write(raw); err != nil {
		return fmt.Errorf("stdio mcp: write: %w", err)
	}
	return nil
}

// readLoop pulls one Response per line and dispatches to deliver.
// Runs until EOF or a fatal stdio error, then signals death once.
func (t *stdioTransport) readLoop() {
	for {
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			t.deliver.onTransportDead(fmt.Errorf("read mcp stdout: %w", err))
			return
		}
		var resp jsonrpcResp
		if err := json.Unmarshal(line, &resp); err != nil {
			// Could be a malformed line — log to stderr so operators
			// see protocol drift, but don't kill the connection.
			fmt.Fprintf(os.Stderr, "mcpbridge: bad mcp response line: %v\n", err)
			continue
		}
		if resp.ID == nil {
			t.deliver.onNotification(line)
			continue
		}
		t.deliver.onResponse(&resp)
	}
}

// close best-effort terminates the MCP child: close stdin (the spec
// recommendation for graceful shutdown), wait briefly, then SIGKILL.
// Idempotent.
func (t *stdioTransport) close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.mu.Unlock()

	_ = t.stdin.Close()
	done := make(chan struct{})
	go func() {
		_ = t.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		<-done
	}
	// Read loop already reported the EOF via onTransportDead.
}
