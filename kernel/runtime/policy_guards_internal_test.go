// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/platform/policy/guards"
)

// The guards moved to the platform in W3.74. These tests pin the daemon's
// half of the move: the daemon ASKS the platform's rules, and it does not
// grow its own copy of them.

// TestTheGuardRulesLiveInThePlatform is the ratchet: the daemon's policy hook
// must not creep the guards' conditions back. The rules are the platform's
// (kernel/platform/policy/guards); the daemon keeps the doors.
func TestTheGuardRulesLiveInThePlatform(t *testing.T) {
	src, err := os.ReadFile("policy.go")
	if err != nil {
		t.Fatalf("read policy.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "guards.Evaluate(") {
		t.Fatal("policy.go must ask the platform's guards")
	}
	for _, forbidden := range []string{
		"EpistemicEscalation &&",
		"IntentRegretGating &&",
		"PromptInjectionGuard !=",
		"PromptInjectionGuard ==",
		"guardRaised",
		"prompt-injection guard: effectful action",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("policy.go grew the guard rule %q back; the rules are the platform's", forbidden)
		}
	}
}

// TestThePromptInjectionModeIsThePlatformsMode pins the alias: the daemon's
// mode and its operator-facing names are the platform's, so the guard and its
// setting can never drift apart.
func TestThePromptInjectionModeIsThePlatformsMode(t *testing.T) {
	if PromptInjectionOn != guards.PromptInjectionOn {
		t.Fatalf("PromptInjectionOn = %v, want the platform's %v", PromptInjectionOn, guards.PromptInjectionOn)
	}
	if PromptInjectionWarn != guards.PromptInjectionWarn {
		t.Fatalf("PromptInjectionWarn = %v, want the platform's %v", PromptInjectionWarn, guards.PromptInjectionWarn)
	}
	if PromptInjectionOff != guards.PromptInjectionOff {
		t.Fatalf("PromptInjectionOff = %v, want the platform's %v", PromptInjectionOff, guards.PromptInjectionOff)
	}
	if got := ParsePromptInjectionMode("off"); got != guards.PromptInjectionOff {
		t.Fatalf("ParsePromptInjectionMode(off) = %v, want the platform's off", got)
	}
	if got := ParsePromptInjectionMode("block"); got != guards.PromptInjectionOn {
		t.Fatalf("ParsePromptInjectionMode(block) = %v, want the platform's on", got)
	}
	if got := ParsePromptInjectionMode("nonsense"); got != guards.PromptInjectionWarn {
		t.Fatalf("ParsePromptInjectionMode(nonsense) = %v, want the platform's warn", got)
	}
}
