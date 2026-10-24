// SPDX-License-Identifier: MIT

// Package approvalapi is the contract layer's home for the approval verdict:
// the operator's answer to a pending request.
//
// The row lives here because the tool that asks for a promotion is a PLUGIN,
// and a plugin may reach the contract and the platform layers and nothing
// else. The approval registry itself — the queue, the timeout, the event —
// stays in the platform (kernel/approval); the platform keeps the name its
// own callers use as an **alias**, so there is one row and one spelling of
// it.
//
// The verdict is a plain string row with a closed vocabulary of four, and
// the vocabulary is the whole of it: granted, denied, timed out, cancelled.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No queue, no clock, no bus.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package approvalapi

// Decision is the operator's verdict on a pending Request.
type Decision string

const (
	// DecisionGrant: the operator approved the request.
	DecisionGrant Decision = "grant"
	// DecisionDeny: the operator refused the request.
	DecisionDeny Decision = "deny"
	// DecisionTimeout: synthesised when the default timeout fires.
	DecisionTimeout Decision = "timeout"
	// DecisionCancel: synthesised when the caller's context is cancelled.
	DecisionCancel Decision = "cancel"
)

// IsTerminal reports whether d is a final outcome (any of the four).
func (d Decision) IsTerminal() bool {
	switch d {
	case DecisionGrant, DecisionDeny, DecisionTimeout, DecisionCancel:
		return true
	}
	return false
}
