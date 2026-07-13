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

	"github.com/agezt/agezt/internal/apperrors"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
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
		return nil, apperrors.WrapSimple("controlplane: read addr", err)
	}
	// Token resolution: AGEZT_TOKEN overrides the on-disk primary token, so a
	// tenant operator can present their tenant token (M38) — `AGEZT_TOKEN=<tok>
	// agt --tenant X edict show` connects to the primary control plane but
	// authenticates as tenant X. Falls back to the daemon's primary token file
	// (the single-tenant default). A missing file is only an error when no env
	// override is set.
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "TOKEN"))
	if token == "" {
		tokenBytes, rerr := os.ReadFile(tokenPath)
		if rerr != nil {
			return nil, apperrors.WrapSimple("controlplane: read token", rerr)
		}
		token = strings.TrimSpace(string(tokenBytes))
	}
	return &Client{
		addr:  strings.TrimSpace(string(addrBytes)),
		token: token,
	}, nil
}

// ProbeExisting reports whether a live daemon is already serving at the
// address recorded in <baseDir>/runtime. It is the single-instance guard the
// daemon runs before claiming the runtime files: a second daemon on the same
// base dir would overwrite addr/token and silently split clients across two
// kernels writing the same journal — each `agt` call would reach whichever
// daemon wrote the addr file last.
//
// Returns:
//   - (addr, true)  — a daemon answered a status probe; do NOT start another.
//   - (addr, false) — an addr file exists but nothing live answers (a stale
//     leftover from a crash); safe to overwrite.
//   - ("",  false)  — no addr file; no daemon.
func ProbeExisting(baseDir string) (addr string, alive bool) {
	c, err := NewClient(baseDir)
	if err != nil {
		return "", false // no runtime files → no daemon recorded
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if _, err := c.Call(ctx, CmdStatus, nil); err != nil {
		return c.addr, false // recorded but unreachable → stale
	}
	return c.addr, true
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
	// Context-aware read: propagate cancellation to the blocking read so
	// Call returns promptly when ctx is done (CWE-666, M992). Without this
	// the read blocks indefinitely on a silent server and Call never
	// returns even after the context expires.
	if err := setReadDeadlineFromCtx(conn, ctx); err != nil {
		return nil, err
	}
	resp, err := readOneResponse(conn)
	if err != nil {
		// Map i/o timeout to context error so callers get DeadlineExceeded.
		if ctxErr := ctx.Err(); ctxErr != nil && isNetTimeout(err) {
			return nil, ctxErr
		}
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

// CallRaw is Call's byte-preserving sibling: it returns the response's
// "result" object as raw JSON instead of a decoded map[string]any. Use it
// when the result carries data whose exact byte form matters — notably
// journaled events, whose BLAKE3 hash is computed over the canonical payload
// bytes and would NOT survive a round-trip through map[string]any (which
// reorders object keys and renumbers integers). The journal-export bundle
// (M101) relies on this so the exported events still re-verify offline.
func (c *Client) CallRaw(ctx context.Context, cmd string, args map[string]any) (json.RawMessage, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := writeRequest(conn, cmd, args, c.token); err != nil {
		return nil, err
	}
	if err := setReadDeadlineFromCtx(conn, ctx); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && isNetTimeout(err) {
			return nil, ctxErr
		}
		return nil, apperrors.Wrap(ctx, "controlplane: read", err)
	}
	var resp struct {
		Type   string          `json:"type"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, apperrors.Wrap(ctx, "controlplane: parse response", err)
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
	if err := setReadDeadlineFromCtx(conn, ctx); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil && isNetTimeout(err) {
				return nil, ctxErr
			}
			return nil, apperrors.Wrap(ctx, "controlplane: read", err)
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return nil, apperrors.Wrap(ctx, "controlplane: parse response", err)
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
			return apperrors.Wrap(ctx, "controlplane: read", err)
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return apperrors.Wrap(ctx, "controlplane: parse response", err)
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

// setReadDeadlineFromCtx sets a read deadline on conn derived from the
// context's deadline (if any). If the context has no deadline, no
// deadline is set (the read stays blocking — existing behaviour). When
// the context has already expired the deadline is set to the past,
// causing an immediate i/o timeout on the next read.
func setReadDeadlineFromCtx(conn net.Conn, ctx context.Context) error {
	if dl, ok := ctx.Deadline(); ok {
		return conn.SetReadDeadline(dl)
	}
	return nil
}

// isNetTimeout reports whether err is a net-level timeout (as opposed to
// a clean EOF or a reset). Used to translate i/o timeouts into context
// errors so callers see context.DeadlineExceeded / context.Canceled.
func isNetTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
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
		return apperrors.WrapSimple("controlplane: marshal request", err)
	}
	enc = append(enc, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(enc); err != nil {
		return apperrors.WrapSimple("controlplane: write", err)
	}
	return nil
}

func readOneResponse(conn net.Conn) (*Response, error) {
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, apperrors.WrapSimple("controlplane: read", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, apperrors.WrapSimple("controlplane: parse response", err)
	}
	return &resp, nil
}

// toString safely extracts a string from a map.
func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// toBool safely extracts a bool from a map.
func toBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// UpdateCheckResult is the result of an update check.
type UpdateCheckResult struct {
	Current  string
	UpToDate bool
	Update   *UpdateInfo
	Disabled bool // true when the daemon has update disabled
}

// UpdateInfo describes an available update.
type UpdateInfo struct {
	Version string
	SHA256  string
	URL     string
	Notes   string
}

// UpdateCheck queries the daemon's update source and returns whether an update
// is available. Returns Update.Disabled=true when the daemon has no update
// source configured.
func (c *Client) UpdateCheck(ctx context.Context) (*UpdateCheckResult, error) {
	out, err := c.Call(ctx, CmdUpdateCheck, nil)
	if err != nil {
		return nil, err
	}
	result := &UpdateCheckResult{
		Current:  toString(out["current"]),
		UpToDate: toBool(out["up_to_date"]),
	}
	if update, ok := out["update"].(map[string]any); ok && update != nil {
		result.Update = &UpdateInfo{
			Version: toString(update["version"]),
			SHA256:  toString(update["sha256"]),
			URL:     toString(update["url"]),
			Notes:   toString(update["notes"]),
		}
	}
	if disabled, ok := out["status"].(string); ok && strings.Contains(disabled, "disabled") {
		result.Disabled = true
	}
	return result, nil
}

// UpdateApplyResult is the result of an update apply.
type UpdateApplyResult struct {
	Applied bool
	Version string
	Error   string
}

// UpdateApply downloads the specified update, validates it, drains the daemon,
// and atomically swaps the binary. The daemon then restarts with the new binary.
func (c *Client) UpdateApply(ctx context.Context, version, sha256, url, notes string) (*UpdateApplyResult, error) {
	args := map[string]any{
		"version": version,
		"sha256":  sha256,
		"url":     url,
	}
	if notes != "" {
		args["notes"] = notes
	}
	out, err := c.Call(ctx, CmdUpdateApply, args)
	if err != nil {
		return nil, err
	}
	return &UpdateApplyResult{
		Applied: toBool(out["applied"]),
		Version: toString(out["version"]),
		Error:   toString(out["error"]),
	}, nil
}

// Close releases the client. Connections are dialed per call (Call/Stream
// each open and close their own), so there is nothing persistent to tear
// down — Close exists so callers can `defer cl.Close()` uniformly and a
// future pooled transport has a hook.
func (c *Client) Close() error { return nil }
