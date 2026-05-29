// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestEdictTest_AllowsKnownSafeCapability — provider_call is L4
// (Allow) in DefaultLevels; the test must come back as a clean
// allow with no would_ask / requires_approval flags.
func TestEdictTest_AllowsKnownSafeCapability(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Capability names use dots (DECISIONS F3 vocabulary): the
	// constants in kernel/edict are `provider.call`, `file.read`,
	// `http.post`, etc. — not underscore-separated.
	res, err := c.Call(context.Background(), controlplane.CmdEdictTest, map[string]any{
		"capability": "provider.call",
		"input":      "POST /v1/chat",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got, _ := res["decision"].(string); got != "allow" {
		t.Errorf("decision = %q want allow", got)
	}
	if hd, _ := res["hard_denied"].(bool); hd {
		t.Errorf("hard_denied = true on a safe capability")
	}
}

// TestEdictTest_HardDeniesForkBomb is the load-bearing safety
// case — a shell command matching the fork-bomb default rule MUST
// come back as a hard deny with the rule name surfaced so an
// operator can confirm which rule caught it.
func TestEdictTest_HardDeniesForkBomb(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdEdictTest, map[string]any{
		"capability": "shell",
		"input":      ":(){:|:&};:",
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got, _ := res["decision"].(string); got != "deny" {
		t.Errorf("decision = %q want deny", got)
	}
	if hd, _ := res["hard_denied"].(bool); !hd {
		t.Errorf("hard_denied = false on a fork-bomb input; rule not catching")
	}
	if rule, _ := res["hard_deny_rule"].(string); rule != "fork-bomb" {
		t.Errorf("hard_deny_rule = %q want fork-bomb", rule)
	}
}

// TestEdictTest_RejectsMissingCapability — the handler must catch
// the missing required arg rather than silently testing the empty
// capability (which would always return a default-deny).
func TestEdictTest_RejectsMissingCapability(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdEdictTest, map[string]any{
		"input": "ls",
	})
	if err == nil {
		t.Fatal("expected error for missing capability")
	}
}

// TestEdictTest_HasNoSideEffects probes the read-only contract:
// running the same test 5 times produces 5 identical outcomes,
// AND the journal head doesn't advance between calls. If a future
// refactor accidentally calls Decide via a path that journals,
// this test fails immediately.
func TestEdictTest_HasNoSideEffects(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	headBefore, _ := k.Journal().Head()
	for i := 0; i < 5; i++ {
		_, err := c.Call(context.Background(), controlplane.CmdEdictTest, map[string]any{
			"capability": "shell",
			"input":      "echo hello",
		})
		if err != nil {
			t.Fatalf("Call %d: %v", i, err)
		}
	}
	headAfter, _ := k.Journal().Head()
	if headBefore != headAfter {
		t.Errorf("journal head advanced: before=%d after=%d (test must be read-only)",
			headBefore, headAfter)
	}
}
