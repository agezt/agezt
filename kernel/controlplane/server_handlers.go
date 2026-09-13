// SPDX-License-Identifier: MIT
//
// Control-plane connection plumbing: handleConn + writeResp + recoverConn
// (the per-connection loop, the response writers, and the panic containment).
// The command handlers (handleVersion + handleHalt + handleCancelRun +
// handleResume + handleWhy + handleWhoami + handleVerify + handleApprovals)
// live in server_commands.go.
// Extracted from server_handlers.go during the Day-206 god-file split.
// Public API unchanged.
package controlplane

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"encoding/json"
)

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Contain a panic ANYWHERE in this connection's handling — including the
	// pre-auth read/parse phase below — to THIS connection: an unrecovered panic
	// in this goroutine crashes the whole process, taking down every in-flight run
	// and channel. A single malformed/edge-case request must not be a daemon-wide
	// DoS — mirror net/http's per-request recover for the control plane's custom
	// TCP protocol. Deferred before parsing; recoverConn reads req.ID at panic time
	// (empty before the request is parsed). Must be deferred directly so its
	// recover() takes effect.
	var req Request
	defer s.recoverConn(conn, &req)

	// Generous read deadline per request — runs can take minutes.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Minute))

	reader := bufio.NewReader(conn)
	// Bounded read (M188): the request line is read BEFORE authentication
	// (the token is inside it), so any local client that can reach the
	// loopback port can stream bytes here. A plain ReadBytes('\n') grows
	// without limit until a newline, so a client that never sends one
	// drives the daemon to OOM — a pre-auth DoS. Cap the request and
	// reject anything larger.
	line, err := readBoundedLine(reader, maxRequestBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			s.writeResp(conn, Response{Type: RespError, Error: "request too large"})
		}
		return
	}
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "bad request: " + err.Error()})
		return
	}
	// Authentication + authorization (M38). The primary (admin) token
	// authorizes everything, on any tenant. Otherwise the request must name a
	// tenant AND present that tenant's own token: the principal is then that
	// tenant, restricted to the TenantAllowed subset of the command registry
	// and pinned to its own tenant. This completes M14 tenant isolation on the
	// control side — a tenant manages its own runs/policy without the primary
	// token, and cannot touch another tenant or daemon-global state.
	primary := s.tokenIsPrimary(req.Token)
	var tenantID string
	if !primary {
		reqTenant, _ := req.Args["tenant"].(string)
		reqTenant = strings.TrimSpace(reqTenant)
		if s.tenants == nil || reqTenant == "" || !s.tenants.Authorize(reqTenant, req.Token) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unauthorized"})
			return
		}
		tenantID = reqTenant
	}

	// Command lookup happens AFTER the auth gate, and the tenant allowlist is
	// applied BEFORE unknown commands are distinguished: a tenant token probing
	// an unregistered command gets the same deny-by-default "forbidden" the
	// legacy tenantTokenAllows gate produced, not an "unknown command" oracle.
	spec, known := commandRegistry[req.Cmd]
	if !primary {
		if !known || !spec.TenantAllowed {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("forbidden: a tenant token cannot run %q (primary token required)", req.Cmd)})
			return
		}
		// Pin the tenant arg to the authorized tenant (defense in depth — it
		// already matched, but no handler should ever see a different value).
		if req.Args == nil {
			req.Args = map[string]any{}
		}
		req.Args["tenant"] = tenantID
	}
	if !known {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown command: " + req.Cmd})
		return
	}

	dc := &DispatchCtx{Ctx: ctx, Conn: conn, Req: req, S: s, K: s.k, Tenant: tenantID, Primary: primary}
	if spec.TenantRouted {
		// Resolve the request's kernel once at the dispatch boundary. For a
		// primary caller without a tenant arg kernelFor("") == s.k, so
		// tenant-routed commands cost nothing extra in the common case.
		k, err := s.kernelFor(tenantOf(req))
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		dc.K = k
	}
	if spec.Streaming == StreamLive {
		// Long-lived streaming work: tie the handler's context to the client
		// connection so a disconnect cancels the (model-heavy) work instead of
		// spending it into a closed connection. Handlers receive the tied
		// context as their ctx parameter and must NOT call cancelOnConnClose
		// themselves — two goroutines reading one conn would race.
		cctx, cancel := cancelOnConnClose(ctx, conn)
		defer cancel()
		dc.Ctx = cctx
	}
	spec.Handler(dc)
}

func (s *Server) writeResp(conn net.Conn, resp Response) {
	_ = writeResp(conn, resp)
}

// recoverConn is deferred per connection: it turns a panic ANYWHERE in the
// connection's handling — the pre-auth parse phase as well as command handling —
// into an error response to the caller instead of an unrecovered goroutine panic
// that would crash the daemon. It takes a *Request (not an id) and reads the id
// at panic time so it can be deferred at the very top of handleConn, before the
// request has been parsed; an early panic simply carries an empty id. Best-effort
// — if the connection is already broken the error write is dropped; the
// load-bearing guarantee is that the process survives so other connections, runs,
// and channels keep working. recoverConn must be deferred DIRECTLY (not wrapped in
// a closure) so its recover() actually stops the panic.
func (s *Server) recoverConn(conn net.Conn, req *Request) {
	if r := recover(); r != nil {
		var id string
		if req != nil {
			id = req.ID
		}
		s.writeResp(conn, Response{ID: id, Type: RespError, Error: "internal error"})
	}
}

// writeResp is the underlying writer that *returns* its error.
// Used by long-lived handlers (pulse) where a broken pipe is the
// client-disconnect signal that should stop the goroutine; the
// method form above keeps the fire-and-forget shape every other
// handler relies on.
func writeResp(conn net.Conn, resp Response) error {
	enc, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	enc = append(enc, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err = conn.Write(enc)
	return err
}

