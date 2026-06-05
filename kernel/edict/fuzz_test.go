// SPDX-License-Identifier: MIT

package edict

import "testing"

// FuzzDecide hardens the trust-ladder decision path — the security policy gate —
// against arbitrary tool input. The input is decoded (JSON), whitespace-collapsed,
// and matched against the hard-deny floor, so it is an untrusted-input parser.
// Three invariants:
//
//   - Decide / DecideWithCeiling never panic on any (capability, input).
//   - The hard-deny floor is UN-OVERRIDABLE: if an input hard-denies, it stays
//     hard-denied (Decision=Deny) at every ceiling — the floor a ceiling cannot
//     loosen (SPEC-12 trust ladder; the M173/M426 evasion-hardening invariant).
//   - A trust ceiling can only TIGHTEN: a lower ceiling is never less strict than
//     a higher one.
func FuzzDecide(f *testing.F) {
	f.Add("shell.exec", "rm -rf /")
	f.Add("shell.exec", `{"command":"rm -rf /"}`)
	f.Add("shell.exec", "rm  -rf   /")
	f.Add("shell.exec", ":(){ :|:& };:")
	f.Add("net.fetch", "https://example.com")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, capability, input string) {
		cap := Capability(capability)
		// Configure the fuzzed capability at the most permissive level over the
		// default hard-deny floor, with AskDeny so Ask-class folds to a clean Deny
		// (making the strictness ordering observable). This is the worst case for the
		// floor: a capability that WOULD auto-allow, so the floor must still bite.
		e := New(Options{
			Levels:    map[Capability]TrustLevel{cap: LevelAllow},
			HardDeny:  DefaultHardDeny(),
			AskPolicy: AskDeny,
		})

		// Invariant 1: never panics.
		base := e.Decide(cap, input)

		// Invariant 2: a hard-deny is un-overridable by any ceiling.
		if base.HardDenied {
			for c := LevelDeny; c <= LevelAllow; c++ {
				o := e.DecideWithCeiling(cap, input, c)
				if !o.HardDenied || o.Decision != DecisionDeny {
					t.Errorf("hard-deny floor overridden at ceiling %v: %+v (input=%q)", c, o, input)
				}
			}
		}

		// Invariant 3: ceiling only tightens — a lower ceiling is never strictly
		// less strict than a higher one. With AskDeny, strictness is just Deny=1.
		strict := func(o Outcome) int {
			if o.Decision == DecisionDeny {
				return 1
			}
			return 0
		}
		for lo := LevelDeny; lo <= LevelAllow; lo++ {
			for hi := lo; hi <= LevelAllow; hi++ {
				if strict(e.DecideWithCeiling(cap, input, lo)) < strict(e.DecideWithCeiling(cap, input, hi)) {
					t.Errorf("lower ceiling %v less strict than higher %v (cap=%q input=%q)", lo, hi, capability, input)
				}
			}
		}
	})
}
