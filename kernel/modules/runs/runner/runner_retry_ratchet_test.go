// SPDX-License-Identifier: MIT

package runner

// The run engine's RETRY RULES door (W3.76ai).
//
// How a failed run is classified, whether the policy allows another attempt
// and how long the backoff waits are the RUNS MODULE's own
// (kernel/modules/runs/retry) — W3.76f moved them there for the delegation
// engine, which could not reach the daemon's unexported copies.
//
// The daemon held three verbs over them (RetryReason, AgentRetryable,
// RetryDelay) and forwarded to the very package next door, so the run engine
// asked the HOST for a rule the module keeps: a round trip inside one module,
// three times more.
//
// The engine asks the module now, the way the delegation engine always has.
//
// These tests pin that: the port carries no retry entry, and the engine calls
// the module's own rules.

import (
	"os"
	"regexp"
	"testing"

	"github.com/agezt/agezt/kernel/modules/runs/retry"
)

// theRetryEntriesAreNotTheHosts are the retry-rule operations the run
// engine's host port must NOT carry: the rules are the module's, and a port
// entry that is a round trip inside one module is a promise the host never
// had to keep.
var theRetryEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\t(RetryReason|AgentRetryable|RetryDelay)\(`)

// TestTheRunEnginePortCarriesNoRetryEntry pins the PORT: it promises the
// host's capabilities, not the module's own retry rules.
func TestTheRunEnginePortCarriesNoRetryEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if theRetryEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a retry entry — the retry rules are the module's (W3.76ai)")
	}
}

// TestTheRunEngineAsksTheModulesRetryRules pins the wiring: the engine calls
// the module's rules, not a host door.
func TestTheRunEngineAsksTheModulesRetryRules(t *testing.T) {
	for _, call := range []string{
		"retry.Reason(",
		"retry.Retryable(",
		"retry.Delay(",
	} {
		if !runnerPackageContains(t, call) {
			t.Errorf("the run engine must ask the module's retry rules (%s) — the rules are the module's (W3.76ai)", call)
		}
	}
}

// TestTheHaltSentinelIsTheModulesOwn pins the one parameter the rules take:
// the daemon owns the IDENTITY of its halt error, and the module asks for it.
// The runner already holds the module's own sentinel (the same value the
// daemon's alias points at), so a halted run is classified exactly as the
// daemon classifies it.
func TestTheHaltSentinelIsTheModulesOwn(t *testing.T) {
	if got := retry.Reason(ErrHalted, ErrHalted); got != "halted" {
		t.Fatalf("the module's sentinel must classify as halted, got %q", got)
	}
}
