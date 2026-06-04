// SPDX-License-Identifier: MIT

// Package pulse implements the proactive heart (SPEC-03): a second heartbeat
// that triggers itself and, on every beat, asks "what changed? · is it
// important? · should I act or tell the user?". This is what makes Agezt a
// Jarvis rather than a tool.
//
// Pulse v1 (ROADMAP §2.1 item 6 / SPEC-03 §9 MVP cut) ships the spine:
//
//	tick → ① observers → ② salience → ③ initiative → ④ briefing
//
// each stage emitting its own journaled event so `agt why` reconstructs the
// whole proactive chain. Deliberately scoped to the Phase 3 demo gate
// (unprompted CI-broken detection → brief to CLI/log → explainable →
// haltable). Wired beyond that gate since: channel brief delivery (the engine's
// BriefSink, fanned out to the Telegram/Slack/Discord/webhook/email sinks the
// daemon wires) and world-model relevance in salience (Config.Relevance, the
// SPEC-05 §3.4 boost). Still deferred: Chronos, standing orders, adaptive
// cadence, autonomous `act`, reflection.
//
// The engine owns no permissions of its own — it borrows the same bus,
// Warden, state store, and provider every other path uses (SPEC-03 §5.1).
package pulse

import "context"

// Severity is an observer-attached hint about how much a delta matters. It is
// only a hint; Salience makes the actual importance call (SPEC-03 §3.3).
type Severity string

const (
	SevLow      Severity = "low"
	SevMedium   Severity = "medium"
	SevHigh     Severity = "high"
	SevCritical Severity = "critical"
)

// Delta is a meaningful change an observer detected — never raw data
// (SPEC-03 §3.1). Observers do their own first-pass filtering so the bus
// isn't flooded.
type Delta struct {
	Source  string            // "probe:ci", "system:disk", ...
	Kind    string            // "probe_failed", "disk_low", ...
	Summary string            // human-readable, one line
	Before  string            // prior known state (for novelty/magnitude)
	After   string            // new state
	RawRef  string            // pointer to detail (not inlined)
	Hints   map[string]string // severity + other observer-known hints
}

// Severity reads the observer's severity hint, defaulting to medium.
func (d Delta) Severity() Severity {
	switch Severity(d.Hints["severity"]) {
	case SevLow:
		return SevLow
	case SevHigh:
		return SevHigh
	case SevCritical:
		return SevCritical
	default:
		return SevMedium
	}
}

// IssueKey identifies "the same underlying issue" for novelty suppression and
// briefing dedupe (SPEC-03 §6.3). Observers can pin it via Hints["issue_key"];
// otherwise it derives from source+kind so repeated identical failures
// coalesce rather than ping twice.
func (d Delta) IssueKey() string {
	if k := d.Hints["issue_key"]; k != "" {
		return k
	}
	return d.Source + "/" + d.Kind
}

// Observer watches one thing and reports meaningful deltas (SPEC-03 §3). The
// set is open; new observer = small implementation. An observer NEVER decides
// importance — that's Salience's job.
type Observer interface {
	// Name identifies the observer (used in events and `agt pulse status`).
	Name() string
	// Poll detects change since the last call and returns zero or more
	// deltas. It must honor ctx and do its own first-pass filtering.
	Poll(ctx context.Context) ([]Delta, error)
}

// Disposition is Salience's recommendation for what should happen to a delta
// (SPEC-03 §4.2).
type Disposition string

const (
	DispDrop   Disposition = "drop"   // journal only; never pushed
	DispDigest Disposition = "digest" // batch into the next briefing
	DispNotify Disposition = "notify" // send soon, normal priority
	DispAlert  Disposition = "alert"  // send now, high priority
	DispAct    Disposition = "act"    // consider autonomous action (v1: → ask/inform)
)

// Dial is the single high-level salience knob (SPEC-03 §4.3). It maps
// dispositions to what actually reaches the user.
type Dial string

const (
	DialQuiet    Dial = "quiet"    // only alert/act reach you
	DialBalanced Dial = "balanced" // notify and up reach you (default)
	DialChatty   Dial = "chatty"   // digest surfaces too
)

// ParseDial normalizes a dial string, defaulting to balanced.
func ParseDial(s string) Dial {
	switch Dial(s) {
	case DialQuiet:
		return DialQuiet
	case DialChatty:
		return DialChatty
	default:
		return DialBalanced
	}
}

// Score is Salience's verdict on one delta (SPEC-03 §4.2).
type Score struct {
	Value       float64     // 0..1
	Reason      string      // why this score
	Disposition Disposition // recommended handling
}

// Delivery is the effective routing after the dial + quiet-hours gate.
type Delivery string

const (
	DeliverNow    Delivery = "now"    // compose + send a brief immediately
	DeliverDigest Delivery = "digest" // accumulate for the next digest flush
	DeliverDrop   Delivery = "drop"   // journal only, nothing sent
)
