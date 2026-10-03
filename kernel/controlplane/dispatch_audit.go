// SPDX-License-Identifier: MIT

package controlplane

// The audit step of the operation pipeline (architecture/20 §3.1, W2.1):
// every command not declared ReadOnly is journaled by dispatch — op.invoked
// before its handler runs, op.completed or op.failed after. Until this,
// auditing was each handler's own business, and about forty state-changing
// commands (provider keys, config-center entries and ACLs, schedules,
// routing, data-lake writes, deletions) left no trace in the journal.

import (
	"net"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// auditedConn records the terminal response a handler writes, so dispatch can
// journal the outcome without each handler reporting it.
type auditedConn struct {
	net.Conn
	mu      sync.Mutex
	settled bool
	errMsg  string
}

func (c *auditedConn) record(resp Response) {
	if resp.Type != RespResult && resp.Type != RespError {
		return // progress events are not the outcome
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settled = true
	c.errMsg = ""
	if resp.Type == RespError {
		c.errMsg = resp.Error
		if c.errMsg == "" {
			c.errMsg = "error"
		}
	}
}

func (c *auditedConn) outcome() (settled bool, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settled, c.errMsg
}

// auditValueMax bounds a journaled string argument.
const auditValueMax = 160

// secretArgFragments mark an argument whose value must never be journaled.
// The bus redactor scrubs secrets it already knows, but a key arriving in
// this very request (provider_key_add, an OAuth code) is not known yet.
var secretArgFragments = []string{
	"key", "token", "secret", "password", "passwd", "passphrase", "cred",
	"auth", "cookie", "value", "private", "signature", "code", "state",
}

func secretArg(name string) bool {
	n := strings.ToLower(name)
	for _, frag := range secretArgFragments {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return false
}

// auditArgs summarizes request arguments for the journal: scalars verbatim
// (strings truncated), secret-named arguments redacted, and nested values
// reduced to their shape so a secret inside them cannot reach the journal.
func auditArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if secretArg(k) {
			out[k] = "[redacted]"
			continue
		}
		switch x := v.(type) {
		case nil:
		case string:
			if utf8.RuneCountInString(x) > auditValueMax {
				x = string([]rune(x)[:auditValueMax]) + "…"
			}
			out[k] = x
		case bool, float64, int, int64:
			out[k] = x
		case []any:
			out[k] = "[list]"
		case map[string]any:
			keys := make([]string, 0, len(x))
			for kk := range x {
				keys = append(keys, kk)
			}
			sort.Strings(keys)
			out[k] = "[object: " + strings.Join(keys, ",") + "]"
		default:
			out[k] = "[value]"
		}
	}
	return out
}

// opAudit journals one state-changing command's invocation and outcome.
type opAudit struct {
	k     *runtime.Kernel
	cmd   string
	corr  string
	start time.Time
	conn  *auditedConn
}

func beginOpAudit(dc *DispatchCtx) *opAudit {
	k := dc.K
	if k == nil || k.Bus() == nil {
		return nil
	}
	caller := "operator"
	if !dc.Primary {
		caller = "tenant"
	}
	a := &opAudit{
		k:     k,
		cmd:   dc.Req.Cmd,
		corr:  k.NewCorrelation(),
		start: time.Now(),
		conn:  &auditedConn{Conn: dc.Conn},
	}
	payload := map[string]any{"op": a.cmd, "caller": caller}
	if dc.Tenant != "" {
		payload["tenant"] = dc.Tenant
	}
	if args := auditArgs(dc.Req.Args); args != nil {
		payload["args"] = args
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "op." + a.cmd,
		Kind:          event.KindOpInvoked,
		Actor:         "controlplane",
		CorrelationID: a.corr,
		Payload:       payload,
	})
	dc.Conn = a.conn
	return a
}

// end journals the outcome. A handler that panicked, or returned without a
// terminal response, is recorded as failed: the audit trail must not claim
// success it cannot see.
func (a *opAudit) end(panicked bool) {
	if a == nil {
		return
	}
	settled, errMsg := a.conn.outcome()
	kind := event.KindOpCompleted
	payload := map[string]any{"op": a.cmd, "duration_ms": time.Since(a.start).Milliseconds()}
	switch {
	case panicked:
		kind, payload["error"] = event.KindOpFailed, "internal error"
	case !settled:
		kind, payload["error"] = event.KindOpFailed, "ended without a response (client gone?)"
	case errMsg != "":
		if utf8.RuneCountInString(errMsg) > auditValueMax {
			errMsg = string([]rune(errMsg)[:auditValueMax]) + "…"
		}
		kind, payload["error"] = event.KindOpFailed, errMsg
	}
	_, _ = a.k.Bus().Publish(event.Spec{
		Subject:       "op." + a.cmd,
		Kind:          kind,
		Actor:         "controlplane",
		CorrelationID: a.corr,
		Payload:       payload,
	})
}
