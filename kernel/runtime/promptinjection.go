// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/platform/policy/guards"
)

// PromptInjectionMode selects how the prompt-injection guard handles an
// effectful action downstream of directive-like untrusted content.
//
// The mode is the platform's rule (W3.74): the guard that reads it lives in
// kernel/platform/policy/guards, and the daemon keeps the name as an alias so
// every host and every test keeps speaking the daemon's vocabulary.
type PromptInjectionMode = guards.PromptInjectionMode

const (
	// PromptInjectionOn routes the action to HITL approval.
	PromptInjectionOn = guards.PromptInjectionOn
	// PromptInjectionWarn allows the action but journals a warning (default).
	PromptInjectionWarn = guards.PromptInjectionWarn
	// PromptInjectionOff disables the active intervention entirely.
	PromptInjectionOff = guards.PromptInjectionOff
)

// ParsePromptInjectionMode maps an operator string (AGEZT_PROMPT_INJECTION_GUARD)
// to a mode. "off"/"0"/"false" → Off, "warn"/"warning"/"audit" → Warn, anything
// else (including empty) → Warn. Operators who want a live approval stop on
// directive-like untrusted content set "on"/"block"/"prompt".
//
// The rule is the platform's (W3.74); the daemon keeps the name.
func ParsePromptInjectionMode(s string) PromptInjectionMode {
	return guards.ParseMode(s)
}
