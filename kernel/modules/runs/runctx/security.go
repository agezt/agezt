// SPDX-License-Identifier: MIT

package runctx

import (
	"context"
)

// WithAutoApproveCapabilities marks a set of capabilities to auto-grant when
// the policy would otherwise prompt for HITL approval, for THIS run and every
// sub-agent it spawns (the context value rides the delegation tree). caps is a
// set of edict capability strings (e.g. {"tool.forge","code.exec"}). This is a
// session-scoped operator grant — e.g. the chat "auto-approve Tool Forge for
// this session" toggle when standing up an agent army — NOT a daemon-wide policy
// change. It never overrides a hard-deny (those resolve to deny, not approval).
// Empty leaves the context unchanged.
func WithAutoApproveCapabilities(ctx context.Context, caps map[string]bool) context.Context {
	if len(caps) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyAutoApproveCaps, caps)
}

// MergeAutoApproveCapabilities merges a capability grant into the one the
// context already carries, and answers the merged set.
func MergeAutoApproveCapabilities(ctx context.Context, caps map[string]bool) map[string]bool {
	if len(caps) == 0 {
		return nil
	}
	out := make(map[string]bool, len(caps))
	if existing, ok := ctx.Value(ctxKeyAutoApproveCaps).(map[string]bool); ok {
		for c, on := range existing {
			if on {
				out[c] = true
			}
		}
	}
	for c, on := range caps {
		if on {
			out[c] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AutoApproveCap reports whether capability c is in this run's auto-approve set.
func AutoApproveCap(ctx context.Context, c string) bool {
	if v, ok := ctx.Value(ctxKeyAutoApproveCaps).(map[string]bool); ok {
		return v[c]
	}
	return false
}

// WithTrustedObservations marks a run as one whose untrusted-observation content
// the operator has chosen to trust (e.g. the chat "trust this run's web content"
// toggle). It downgrades the prompt-injection guard from blocking to warn FOR
// THIS RUN and its sub-agents, so a deliberately operator-driven agentic task
// isn't interrupted for every action — without changing the daemon-wide posture
// or touching any hard-deny. Never affects the F4 floor or SSRF/budget guards.
func WithTrustedObservations(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyTrustedObservations, true)
}

// TrustedObservations reports whether this run carries the operator's
// trust-this-run grant.
func TrustedObservations(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyTrustedObservations).(bool)
	return v
}
