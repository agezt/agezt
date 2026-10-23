// SPDX-License-Identifier: MIT

// Package pulseapi is the contract the briefing rows and the surfaces that
// deliver them agree on.
//
// A **brief** is one composed message ready for delivery: the pulse engine
// composes it, and a **sink** delivers it to a surface — the daemon log, a
// chat channel, a webhook. The surfaces are PLUGINS (the built-in channels
// are), and a plugin may reach the contract and the platform layers and
// nothing else (archcheck's plugin rule) — so the row a plugin names, and
// the sink it hands a brief to, have to live here.
//
// The composition does NOT live here: composing a brief from a scored delta,
// the engine's loop, the observers and the salience rules stay in the pulse
// module — they are the implementation. What travels is the row, the
// dispositions it carries and the sinks that deliver it.
//
// The rules of the road:
//
//   - Rows, ports and the rules that belong to the row only.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package pulseapi

import (
	"fmt"
	"io"
	"strings"
)

// Disposition says what happens to a scored delta.
type Disposition string

const (
	DispDrop   Disposition = "drop"   // journal only; never pushed
	DispDigest Disposition = "digest" // batch into the next briefing
	DispNotify Disposition = "notify" // send soon, normal priority
	DispAlert  Disposition = "alert"  // send now, high priority
	DispAct    Disposition = "act"    // consider autonomous action (v1: → ask/inform)
)

// Brief is one composed message ready for delivery (SPEC-03 §6).
type Brief struct {
	Title         string      `json:"title"`
	Body          string      `json:"body,omitempty"`
	Disposition   Disposition `json:"disposition"`
	IssueKey      string      `json:"issue_key"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	Items         int         `json:"items"` // 1 for a single brief, N for a digest
}

// BriefSink delivers a composed brief to a surface. v1 ships LogSink (daemon
// log + journal via the engine); the Telegram sink slots in here in Phase 4
// without touching the engine.
type BriefSink interface {
	Deliver(b Brief) error
}

// LogSink writes a one-line human brief to an io.Writer (the daemon's
// stdout). This is the Phase 3 "brief to CLI/log" delivery.
type LogSink struct{ W io.Writer }

// Deliver implements BriefSink.
func (l LogSink) Deliver(b Brief) error {
	if l.W == nil {
		return nil
	}
	if b.Body != "" {
		_, err := fmt.Fprintf(l.W, "📣 BRIEF [%s] %s\n%s\n", b.Disposition, b.Title, indent(b.Body))
		return err
	}
	_, err := fmt.Fprintf(l.W, "📣 BRIEF [%s] %s\n", b.Disposition, b.Title)
	return err
}

// SinkFunc adapts a plain function to a BriefSink. The daemon uses it to wrap
// a channel's Send into a sink (e.g. brief → Telegram sendMessage) without the
// pulse package depending on any channel.
type SinkFunc func(Brief) error

// Deliver implements BriefSink.
func (f SinkFunc) Deliver(b Brief) error { return f(b) }

// MultiSink fans a brief out to several sinks (e.g. the daemon log AND
// Telegram). Delivery continues to every sink; the first error is returned but
// does not stop the others — a Telegram outage must not suppress the log line.
type MultiSink []BriefSink

// Deliver implements BriefSink.
func (m MultiSink) Deliver(b Brief) error {
	var firstErr error
	for _, s := range m {
		if s == nil {
			continue
		}
		if err := s.Deliver(b); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// indent renders a brief's body as an indented block.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		lines[i] = "    " + ln
	}
	return strings.Join(lines, "\n")
}
