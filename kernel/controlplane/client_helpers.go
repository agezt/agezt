// SPDX-License-Identifier: MIT
//
// Control-plane Client helpers: dial + setReadDeadlineFromCtx +
// mapNetTimeoutToCtxError + isNetTimeout + writeRequest + readOneResponse +
// toString + toBool.
// Extracted from client.go during the Day-202 god-file split.
// Public API unchanged.
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

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

// mapNetTimeoutToCtxError translates a net i/o timeout (from a read whose
// deadline was set from ctx via setReadDeadlineFromCtx) into the matching
// context error so callers see context.DeadlineExceeded / context.Canceled.
//
// Why not just check ctx.Err()? The connection read deadline and the context
// timer are independent mechanisms. Under -count=20 stress (or heavy CPU
// contention) the read deadline can fire a few microseconds before the context
// timer, leaving ctx.Err() == nil even though the timeout was definitively
// caused by the context deadline. Since the read deadline was derived FROM the
// context, an i/o timeout on this read IS the context deadline firing — so we
// return context.DeadlineExceeded regardless of the ctx.Err() race. If there's
// no deadline on the context (no setReadDeadlineFromCtx was called), the error
// is a genuine network issue and we pass it through.
func mapNetTimeoutToCtxError(ctx context.Context, err error) error {
	if !isNetTimeout(err) {
		return fmt.Errorf("controlplane: read: %w", err)
	}
	// The read deadline was derived from ctx, so an i/o timeout here means
	// the ctx deadline expired. ctx.Err() may be nil due to a scheduling
	// race between the two timers.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.DeadlineExceeded
	}
	// No context deadline set — the read timed out for a different reason
	// (e.g. a fixed write deadline in writeRequest). Return as-is.
	return fmt.Errorf("controlplane: read: %w", err)
}

// isNetTimeout reports whether err is a net-level timeout (as opposed to
// a clean EOF or a reset). Used to translate i/o timeouts into context
// errors so callers see context.DeadlineExceeded / context.Canceled.
// We check both errors.As (the proper way) and a string fallback because
// bufio.Reader can wrap the original net error in ways that errors.As
// misses on some Go versions.
func isNetTimeout(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "i/o timeout")
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
