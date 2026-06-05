// SPDX-License-Identifier: MIT

package edict

import "testing"

// TestHardDeny_ForkBombSpacingVariants closes a real evasion in the SPEC-06 §3.2
// hard-deny floor: the canonical fork bomb `:(){ :|:& };:` carries spaces that
// are syntactically optional in bash, but the floor rule is the no-space form
// `:(){:|:&};:`. The matcher collapsed whitespace RUNS to a single space, so the
// spaced (and the actually-valid) fork bomb survived collapse and was NOT
// denied. denyCandidates now also derives a whitespace-STRIPPED candidate, so
// every spacing variant normalizes onto the floor rule.
func TestHardDeny_ForkBombSpacingVariants(t *testing.T) {
	e := New(Options{})
	denied := []string{
		`:(){ :|:& };:`,                   // canonical (spaced) — the one that evaded before
		`:(){:|:&};:`,                     // no-space variant
		`:(){  :|:&  };:`,                 // extra padding
		"bash -c ':(){ :|:& };:'",         // wrapped in a shell invocation
		"{\"command\":\":(){ :|:& };:\"}", // JSON-wrapped (the agent's tool-input shape)
		":(){\t:|:&\t};:",                 // tab-separated
	}
	for _, in := range denied {
		o := e.Decide(CapShell, in)
		if !o.HardDenied {
			t.Errorf("fork bomb NOT hard-denied (evasion): %q", in)
			continue
		}
		if o.HardDenyRule != "fork-bomb" {
			t.Errorf("input %q denied by %q, want fork-bomb", in, o.HardDenyRule)
		}
	}
}

// TestHardDeny_StrippingDoesNotBlockBenign: the whitespace-stripped candidate
// must not turn an ordinary command into a hard-deny. These contain no
// dangerous substring in any whitespace normalization.
func TestHardDeny_StrippingDoesNotBlockBenign(t *testing.T) {
	e := New(Options{})
	allowed := []string{
		"echo hello world",
		"ls -la /home/user",
		"git status --short",
		"cat my notes.txt",
		"grep -r TODO ./src",
	}
	for _, in := range allowed {
		if o := e.Decide(CapShell, in); o.HardDenied {
			t.Errorf("benign command hard-denied by %q: %q", o.HardDenyRule, in)
		}
	}
}

// TestHardDeny_WordSplitNotFalselyDenied: ordinary prose whose words would, under
// FULL whitespace stripping, collapse onto an alphabetic floor rule (mkfs/reboot/
// poweroff/wipefs) must NOT be hard-denied — a hard-deny has no override, so a false
// positive permanently blocks a legitimate command (M426). Punctuation-adjacent
// stripping preserves these while still catching the punctuation-spaced fork bomb.
func TestHardDeny_WordSplitNotFalselyDenied(t *testing.T) {
	e := New(Options{})
	allowed := []string{
		"mk fs.go",                       // full-strip → "mkfs.go" → matched "mkfs"
		"re boot the server politely",    // full-strip → "...reboot..." → matched "reboot"
		"discuss power off mode here",    // full-strip → "...poweroff..." → matched "poweroff"
		"wipe fs metadata in the readme", // full-strip → "...wipefs..." → matched "wipefs"
	}
	for _, in := range allowed {
		if o := e.Decide(CapShell, in); o.HardDenied {
			t.Errorf("word-split prose falsely hard-denied by %q: %q", o.HardDenyRule, in)
		}
	}
}

// TestHardDeny_SpaceBearingRulesStillFire: padding-normalisation for the
// space-bearing floor rules (e.g. `rm -rf /`) must still work — the new stripped
// candidate complements, never replaces, the collapsed one.
func TestHardDeny_SpaceBearingRulesStillFire(t *testing.T) {
	e := New(Options{})
	for _, in := range []string{"rm -rf /", "rm  -rf   /", "sudo rm -rf /"} {
		o := e.Decide(CapShell, in)
		if !o.HardDenied {
			t.Errorf("rm -rf / variant not denied: %q", in)
		}
	}
}
