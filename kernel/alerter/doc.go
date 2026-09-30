// SPDX-License-Identifier: MIT

// Package alerter pushes warning/critical alerts to the configured channels
// (M782). It watches the bus for proactive-signal event kinds (run failures,
// blocked egress, budget/rate trips, halts, and a pending approval — M922) and
// delivers a short brief through the existing Pulse channel sinks — so the
// operator hears about problems, and is asked to approve a blocked run, without
// the console open. (The console surfaces a pending approval through its own
// ApprovalsBell rather than the Alerts view, so the two stay in sync on intent
// even though only the daemon classifies approvals here.)
//
// Pulse-originated kinds (observer.delta, briefing.sent) are deliberately NOT
// handled here: the Pulse engine already delivers its own briefs through the
// same sinks, and notifying them twice would double every heartbeat signal.
package alerter
