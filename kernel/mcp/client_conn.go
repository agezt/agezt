// SPDX-License-Identifier: MIT

// client_conn.go owns the clientConn implementation: the
// child-process / pipe-paired Conn that speaks the JSON-RPC
// wire format, plus every Conn method (readLoop / handshake /
// Tools / Call / Close / roundTrip / send). The public Conn
// surface + Dial live in client.go; env helpers in
// client_helpers.go.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// clientConn is the production Conn: one child process, one reader goroutine
// feeding frames into a channel, calls serialized under mu.
type clientConn struct {
	stdin  io.WriteCloser
	frames chan []byte
	dead   chan struct{} // closed by the reader on EOF/overflow
	stop   func()        // kills the child (idempotent via closeOnce)

	mu     sync.Mutex // serializes calls — one outstanding request id
	nextID atomic.Int64

	tools     []ToolDef
	closeOnce sync.Once
}

// Dial spawns command args... and completes the MCP handshake + tool
// discovery. The child gets a SCRUBBED environment — PATH and friends, never
// AGEZT_* or secret-shaped variables — so a registered server can't read the
// daemon's keys out of its env. The optional env map (M898) is the operator's
// explicit per-server opt-in (e.g. an API token); those entries are injected on
// top of the scrubbed base, so a credentialed server gets exactly what it needs
// without un-scrubbing the daemon's ambient secrets.
func Dial(ctx context.Context, command string, args []string, env map[string]string) (Conn, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = appendEnv(scrubbedEnv(), env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout: %w", err)
	}
	cmd.Stderr = io.Discard // server logs are its own business; protocol rides stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %s: %w", command, err)
	}
	c := newClientConn(stdin, stdout, func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // reap; ignore exit status — we killed it
	})
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	if err := c.handshake(hctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// newClientConn wires a conn over arbitrary pipes — the testable core.
func newClientConn(stdin io.WriteCloser, stdout io.Reader, stop func()) *clientConn {
	c := &clientConn{
		stdin:  stdin,
		frames: make(chan []byte, 16),
		dead:   make(chan struct{}),
		stop:   stop,
	}
	go c.readLoop(stdout)
	return c
}

// readLoop pushes each stdout line into frames until EOF or an oversized
// frame, then signals death.
func (c *clientConn) readLoop(r io.Reader) {
	defer close(c.dead)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxFrameBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		b := make([]byte, len(line))
		copy(b, line)
		select {
		case c.frames <- b:
		case <-time.After(60 * time.Second):
			return // consumer gone — stop reading rather than block forever
		}
	}
	// EOF, an oversized frame, or a read error all end the loop the same way:
	// the deferred close(dead) tells callers the connection is gone. The
	// specific scanner error adds nothing actionable here.
	_ = sc.Err()
}

// handshake: initialize → initialized notification → tools/list.
func (c *clientConn) handshake(ctx context.Context) error {
	var initRes json.RawMessage
	err := c.roundTrip(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": "1"},
	}, &initRes)
	if err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}
	if err := c.send(rpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"}); err != nil {
		return fmt.Errorf("mcp: initialized notification: %w", err)
	}
	var listRes struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := c.roundTrip(ctx, "tools/list", map[string]any{}, &listRes); err != nil {
		return fmt.Errorf("mcp: tools/list: %w", err)
	}
	c.tools = listRes.Tools
	return nil
}

// Tools implements Conn.
func (c *clientConn) Tools() []ToolDef {
	out := make([]ToolDef, len(c.tools))
	copy(out, c.tools)
	return out
}

// Call implements Conn: one tools/call, content flattened to text.
func (c *clientConn) Call(ctx context.Context, tool string, args json.RawMessage) (string, bool, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, callTimeout)
		defer cancel()
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`) // MCP requires an arguments object
	}
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	err := c.roundTrip(ctx, "tools/call", map[string]any{"name": tool, "arguments": json.RawMessage(args)}, &res)
	if err != nil {
		return "", false, err
	}
	var parts []string
	for _, blk := range res.Content {
		if blk.Text != "" {
			parts = append(parts, blk.Text)
		}
	}
	return strings.Join(parts, "\n"), res.IsError, nil
}

// Close implements Conn.
func (c *clientConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close() // polite: EOF lets a well-behaved server exit
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-c.dead:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
		if c.stop != nil {
			c.stop()
		}
	})
	return nil
}

// roundTrip sends one request and waits for ITS response, skipping
// notifications and unrelated ids. Calls are serialized, so at most one id
// is ever outstanding.
func (c *clientConn) roundTrip(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID.Add(1)
	if err := c.send(rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.dead:
			return errors.New("mcp: server connection lost")
		case frame := <-c.frames:
			var resp rpcResponse
			if err := json.Unmarshal(frame, &resp); err != nil || resp.ID == nil || *resp.ID != id {
				continue // notification / unrelated id / junk — skip
			}
			if resp.Error != nil {
				return fmt.Errorf("mcp: %s: server error %d: %s", method, resp.Error.Code, resp.Error.Message)
			}
			if out != nil {
				if err := json.Unmarshal(resp.Result, out); err != nil {
					return fmt.Errorf("mcp: %s: parse result: %w", method, err)
				}
			}
			return nil
		}
	}
}

func (c *clientConn) send(req rpcRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}
