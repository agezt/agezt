// SPDX-License-Identifier: MIT

package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ersinkoc/agezt/kernel/event"
)

// Client connects to a running agezt daemon's control plane.
type Client struct {
	addr  string
	token string
}

// NewClient loads the address and token from <baseDir>/runtime/. Returns
// an error (with hint) if either file is absent — the daemon isn't running.
func NewClient(baseDir string) (*Client, error) {
	addrPath := filepath.Join(baseDir, "runtime", addrFile)
	tokenPath := filepath.Join(baseDir, "runtime", tokenFile)
	addrBytes, err := os.ReadFile(addrPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("controlplane: daemon not running (no %s)", addrPath)
		}
		return nil, fmt.Errorf("controlplane: read addr: %w", err)
	}
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("controlplane: read token: %w", err)
	}
	return &Client{
		addr:  strings.TrimSpace(string(addrBytes)),
		token: strings.TrimSpace(string(tokenBytes)),
	}, nil
}

// ErrServerError wraps a server-side error response.
type ErrServerError struct{ Msg string }

func (e *ErrServerError) Error() string { return "controlplane: " + e.Msg }

// Call sends a non-streaming command and returns the result map.
func (c *Client) Call(ctx context.Context, cmd string, args map[string]any) (map[string]any, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := writeRequest(conn, cmd, args, c.token); err != nil {
		return nil, err
	}
	resp, err := readOneResponse(conn)
	if err != nil {
		return nil, err
	}
	if resp.Type == RespError {
		return nil, &ErrServerError{Msg: resp.Error}
	}
	if resp.Type != RespResult {
		return nil, fmt.Errorf("controlplane: unexpected response type %q", resp.Type)
	}
	return resp.Result, nil
}

// Stream sends a streaming command (currently only "run"). onEvent is
// called for every event before the final result is returned.
func (c *Client) Stream(ctx context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) (map[string]any, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := writeRequest(conn, cmd, args, c.token); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("controlplane: read: %w", err)
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return nil, fmt.Errorf("controlplane: parse response: %w", err)
		}
		switch resp.Type {
		case RespEvent:
			if onEvent != nil && resp.Event != nil {
				onEvent(resp.Event)
			}
		case RespResult:
			return resp.Result, nil
		case RespError:
			return nil, &ErrServerError{Msg: resp.Error}
		default:
			return nil, fmt.Errorf("controlplane: unexpected response type %q", resp.Type)
		}
	}
}

// StreamUntilCancel is Stream's open-ended sibling for commands
// like CmdPulseSubscribe that never send RespResult — the server
// keeps streaming events until either side closes the connection.
//
// Lifecycle: the call returns when:
//
//   - ctx is cancelled (clean shutdown — returns nil),
//   - the server sends RespError (returns *ErrServerError),
//   - the server closes the conn (returns the wrapped read error).
//
// The ctx-cancelled path needs help: net.Conn reads ignore ctx, so
// we spawn a watcher that closes the conn when ctx is done. The
// subsequent Read returns an error, ctx.Err() is non-nil, and we
// return nil to distinguish operator-Ctrl+C from real failure.
func (c *Client) StreamUntilCancel(ctx context.Context, cmd string, args map[string]any, onEvent func(*event.Event)) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := writeRequest(conn, cmd, args, c.token); err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if ctx.Err() != nil {
				return nil // operator cancellation; not a failure
			}
			return fmt.Errorf("controlplane: read: %w", err)
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return fmt.Errorf("controlplane: parse response: %w", err)
		}
		switch resp.Type {
		case RespEvent:
			if onEvent != nil && resp.Event != nil {
				onEvent(resp.Event)
			}
		case RespResult:
			// Server-initiated terminus. Pulse doesn't send this, but
			// future commands sharing the helper might.
			return nil
		case RespError:
			return &ErrServerError{Msg: resp.Error}
		default:
			return fmt.Errorf("controlplane: unexpected response type %q", resp.Type)
		}
	}
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c.addr == "" {
		return nil, errors.New("controlplane: client not initialised")
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	return d.DialContext(ctx, "tcp", c.addr)
}

func writeRequest(conn net.Conn, cmd string, args map[string]any, token string) error {
	req := Request{
		ID:    "q-" + time.Now().Format("150405.000"),
		Cmd:   cmd,
		Token: token,
		Args:  args,
	}
	enc, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("controlplane: marshal request: %w", err)
	}
	enc = append(enc, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(enc); err != nil {
		return fmt.Errorf("controlplane: write: %w", err)
	}
	return nil
}

func readOneResponse(conn net.Conn) (*Response, error) {
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("controlplane: read: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("controlplane: parse response: %w", err)
	}
	return &resp, nil
}
