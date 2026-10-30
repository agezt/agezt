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
	"sync"
	"time"

	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
	"github.com/agezt/agezt/kernel/app/opaudit"
	"github.com/agezt/agezt/kernel/event"
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

// opAudit journals one state-changing command's invocation and outcome.
type opAudit struct {
	ports appeventbus.Ports
	cmd   string
	corr  string
	start time.Time
	conn  *auditedConn
}

func beginOpAudit(dc *DispatchCtx) *opAudit {
	scope := dc.Scope
	ports := scope.BusPorts
	if !ports.Ready() {
		return nil
	}
	caller := "operator"
	if !dc.Primary {
		caller = "tenant"
	}
	a := &opAudit{
		ports: ports,
		cmd:   dc.Req.Cmd,
		corr:  scope.CorrelationPorts.Mint(),
		start: time.Now(),
		conn:  &auditedConn{Conn: dc.Conn},
	}
	payload := map[string]any{"op": a.cmd, "caller": caller}
	if dc.Tenant != "" {
		payload["tenant"] = dc.Tenant
	}
	if args := opaudit.SummarizeArgs(dc.Req.Args); args != nil {
		payload["args"] = args
	}
	_, _ = ports.Publish(event.Spec{
		Subject:       "op." + a.cmd,
		Kind:          event.KindOpInvoked,
		Actor:         "controlplane",
		CorrelationID: a.corr,
		Payload:       payload,
	})
	dc.Conn = a.conn
	dc.CorrelationID = a.corr
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
		kind, payload["error"] = event.KindOpFailed, opaudit.TruncateValue(errMsg)
	}
	_, _ = a.ports.Publish(event.Spec{
		Subject:       "op." + a.cmd,
		Kind:          kind,
		Actor:         "controlplane",
		CorrelationID: a.corr,
		Payload:       payload,
	})
}
