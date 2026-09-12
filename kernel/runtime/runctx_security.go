// SPDX-License-Identifier: MIT

// Run context security: WithAutoApproveCapabilities + mergeAutoApproveCapabilities + autoApproveCap + ParsePromptInjectionMode + WithTrustedObservations + trustedObservations.
// Code extracted from runctx.go during the Day-69 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"strings"
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

func mergeAutoApproveCapabilities(ctx context.Context, caps map[string]bool) map[string]bool {
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

// autoApproveCap reports whether capability c is in this run's auto-approve set.
func autoApproveCap(ctx context.Context, c string) bool {
	if v, ok := ctx.Value(ctxKeyAutoApproveCaps).(map[string]bool); ok {
		return v[c]
	}
	return false
}

// PromptInjectionMode selects how the prompt-injection guard handles an
// effectful action downstream of directive-like untrusted content.
type PromptInjectionMode int

const (
	// PromptInjectionOn routes the action to HITL approval.
	PromptInjectionOn PromptInjectionMode = iota
	// PromptInjectionWarn allows the action but journals a warning (default).
	PromptInjectionWarn
	// PromptInjectionOff disables the active intervention entirely.
	PromptInjectionOff
)

// ParsePromptInjectionMode maps an operator string (AGEZT_PROMPT_INJECTION_GUARD)
// to a mode. "off"/"0"/"false" → Off, "warn"/"warning"/"audit" → Warn, anything
// else (including empty) → Warn. Operators who want a live approval stop on
// directive-like untrusted content set "on"/"block"/"prompt".
func ParsePromptInjectionMode(s string) PromptInjectionMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off", "0", "false", "no":
		return PromptInjectionOff
	case "on", "1", "true", "yes", "block", "prompt":
		return PromptInjectionOn
	case "", "warn", "warning", "audit":
		return PromptInjectionWarn
	default:
		return PromptInjectionWarn
	}
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

// trustedObservations reports whether this run carries the operator's
// trust-this-run grant.
func trustedObservations(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyTrustedObservations).(bool)
	return v
}

// WithAgentProfile applies a roster profile to a run's context (M790): the
// soul becomes the system override, the model + ordered fallbacks become the
// run's model chain, and the memory scope follows the identity (M786). The
// per-run cost ceiling is NOT applied here — callers layer it so their own
// explicit budget wins (mirrors handleRun's precedence). Used by the standing
// runner so an order can fire AS a named agent; handleRun keeps its inline