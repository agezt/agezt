// SPDX-License-Identifier: MIT

package runtime

// The daemon's RETRY RULES door (W3.76ai).
//
// How a failed run is classified, whether the policy allows another attempt
// and how long the backoff waits are the RUNS MODULE's own
// (kernel/modules/runs/retry) — W3.76f moved them there for the delegation
// engine, which could not reach the daemon's unexported copies.
//
// The daemon kept three verbs over them (RetryReason, AgentRetryable,
// RetryDelay) and forwarded to the very package next door, so the run engine
// — which lives inside that module — asked the HOST for a rule the module
// keeps: a round trip inside one module, three times more.
//
// These tests pin that: the daemon holds no verb per retry operation, no
// forwarding wrapper, and no name for the module's rules.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// theRetryVerbsAreNotTheDaemons are the retry-lifecycle verbs the daemon must
// not hold: the retry rules are the module's, and a verb per rule is a door
// the host never had to keep.
var theRetryVerbsAreNotTheDaemons = regexp.MustCompile(`(?m)^func \(k \*Kernel\) (RetryReason|AgentRetryable|RetryDelay)\(`)

// theRetryWrappersAreNotTheDaemons are the daemon's forwarding wrappers: the
// bodies they carried are the module's functions, one call away.
var theRetryWrappersAreNotTheDaemons = regexp.MustCompile(`\b(retryReason|agentRetryable|retryDelay)\(`)

// theRetryRulesAreNotTheDaemons are the module's rules: the classification,
// the allowlist and the backoff.
var theRetryRulesAreNotTheDaemons = regexp.MustCompile(`retry\.(Reason|Retryable|Delay)\(`)

// TestTheDaemonHoldsNoVerbPerRetryRule pins the SURFACE: the daemon answers
// for no retry rule.
func TestTheDaemonHoldsNoVerbPerRetryRule(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theRetryVerbsAreNotTheDaemons.Match(src) {
			t.Errorf("%s declares a retry-rule verb — the retry rules are the module's (W3.76ai)", f)
		}
	}
}

// TestTheDaemonHoldsNoRetryWrapper pins the BODY: the daemon forwards
// nothing — the run engine asks the module's package.
func TestTheDaemonHoldsNoRetryWrapper(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theRetryWrappersAreNotTheDaemons.Match(src) {
			t.Errorf("%s grew a retry wrapper back — the retry rules are the module's (W3.76ai)", f)
		}
	}
}

// TestTheDaemonDoesNotNameTheRetryRules pins the RULES: the daemon does not
// even reach for the module's functions. The delegation engine asks them
// directly; the run engine does too now.
func TestTheDaemonDoesNotNameTheRetryRules(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theRetryRulesAreNotTheDaemons.Match(src) {
			t.Errorf("%s names the module's retry rules — the rules are the module's and the module asks them itself (W3.76ai)", f)
		}
	}
}

// TestTheRunEngineAsksTheModulesRetryRules pins the WIRING from the host's
// side: the run engine is the module's, and the retry rules are next door.
func TestTheRunEngineAsksTheModulesRetryRules(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "modules", "runs", "runner", "runner_retry.go"))
	if err != nil {
		t.Fatalf("read the run engine's retry loop: %v", err)
	}
	for _, required := range []string{
		"retry.Reason(",
		"retry.Retryable(",
		"retry.Delay(",
	} {
		if !strings.Contains(string(src), required) {
			t.Errorf("the run engine must ask the module's retry rules (%s) — the rules are the module's (W3.76ai)", required)
		}
	}
}
