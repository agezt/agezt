// SPDX-License-Identifier: MIT

package mcp

// A minimal MCP (Model Context Protocol) client over stdio: spawn the server
// process, JSON-RPC `initialize` + `notifications/initialized`, discover its
// tools (`tools/list`), forward calls (`tools/call`). Line-delimited JSON-RPC
// frames, size-capped; calls are serialized (one outstanding request), with
// stray notifications and unrelated ids skipped. Deliberately small — the
// 90% of MCP that turns a server into callable tools; resources/prompts/SSE
// can layer on later.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// maxFrameBytes caps one JSON-RPC line from the server (matches the
	// external bridge's M185 bound) so a hostile server can't OOM the daemon.
	maxFrameBytes = 16 << 20
	// handshakeTimeout bounds initialize + tools/list at attach time.
	handshakeTimeout = 15 * time.Second
	// callTimeout bounds one forwarded tools/call when the run's own context
	// carries no earlier deadline.
	callTimeout = 90 * time.Second

	protocolVersion = "2024-11-05"
	clientName      = "agezt"
)

// ToolDef is one tool an attached server advertises.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Conn is a live attachment to one MCP server. Implemented by the stdio
// client here; an interface so the runtime (and tests) never depend on a
// real child process.
type Conn interface {
	// Tools returns the tool list discovered at attach time.
	Tools() []ToolDef
	// Call forwards one tools/call and returns the flattened text content
	// plus the server's own isError verdict.
	Call(ctx context.Context, tool string, args json.RawMessage) (string, bool, error)
	// Close detaches: asks the child to exit, then kills it. Idempotent.
	Close() error
}

// Dialer spawns + handshakes one server. The runtime takes it as a seam so
// tests can attach fakes; Dial is the production implementation. env is the
// server's opt-in extra environment (M898), injected on top of the scrubbed base.
type Dialer func(ctx context.Context, command string, args []string, env map[string]string) (Conn, error)

// jsonrpc wire shapes (only what this client speaks).
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"` // nil = notification
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     *int64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

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
		case <-time.After(5 * time.Second):
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
		select {
		case <-c.dead:
		case <-time.After(2 * time.Second):
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

// appendEnv overlays the operator's explicit per-server env (M898) onto the
// scrubbed base. These are the values the operator typed for THIS server, so they
// are allowed through even when secret-shaped (that's the point) — the scrub still
// governs everything inherited from the daemon's own environment. A later entry
// for the same key wins.
func appendEnv(base []string, extra map[string]string) []string {
	if len(extra) == 0 {
		return base
	}
	out := append([]string(nil), base...)
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

// scrubbedEnv builds the child environment: harmless OS variables only
// (PATH, Windows system vars, per-user dirs npm-style launchers need,
// locale) — never AGEZT_* or secret-shaped variables. Mirrors the code-exec
// sandbox's scrub; this is the load-bearing safety property of attach.
func scrubbedEnv() []string {
	allow := map[string]bool{
		"PATH": true, "PATHEXT": true, "COMSPEC": true,
		"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true,
		"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
		"LANG": true, "HOME": true, "USERPROFILE": true,
		"APPDATA": true, "LOCALAPPDATA": true, "PROGRAMDATA": true,
		"TEMP": true, "TMP": true, "TMPDIR": true,
		"PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "PROGRAMW6432": true,
	}
	var out []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		up := strings.ToUpper(name)
		if isSecretName(up) {
			continue
		}
		if allow[up] || strings.HasPrefix(up, "LC_") {
			out = append(out, kv)
		}
	}
	return out
}

// isSecretName mirrors the code-exec sandbox's rule: anything secret-shaped
// — and the whole AGEZT_* namespace — never reaches a spawned server.
func isSecretName(up string) bool {
	for _, frag := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CRED", "AWS_", "AGEZT_"} {
		if strings.Contains(up, frag) {
			return true
		}
	}
	return false
}
