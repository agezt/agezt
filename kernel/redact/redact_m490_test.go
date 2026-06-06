// SPDX-License-Identifier: MIT

package redact_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/redact"
)

// These tests pin invariants of SetSecrets/Redact that mutation testing
// (go-mutesting, M490) showed the suite did not constrain: any of them could be
// silently weakened by a one-character refactor while every existing test still
// passed. Each asserts a property whose violation would leak a secret.

// The literal-length floor is exactly 8 (minLiteralLen). A literal of exactly 8
// chars must be redacted; one of 7 must be left intact (too likely to be an
// ordinary substring). Kills the `<` → `<=` boundary flip and the
// minLiteralLen 8 → 7 / 8 → 9 constant mutants.
func TestSetSecrets_MinLengthBoundaryIsEight(t *testing.T) {
	r := redact.New()
	// Distinct, non-overlapping, and matching no built-in pattern.
	r.SetSecrets([]string{"PQRSTUVW", "ABCDEFG"}) // 8 chars, 7 chars
	out := r.Redact("a PQRSTUVW b ABCDEFG c")

	if strings.Contains(out, "PQRSTUVW") {
		t.Errorf("an exactly-8-char literal must be redacted (floor is inclusive at 8): %q", out)
	}
	if !strings.Contains(out, "ABCDEFG") {
		t.Errorf("a 7-char literal is below the floor and must be left intact: %q", out)
	}
}

// A leading value that is dropped for being too short must not stop the loop from
// registering the secrets after it. Kills the `continue` → `break` mutant on the
// length-filter branch.
func TestSetSecrets_ShortValueDoesNotStopLaterSecrets(t *testing.T) {
	r := redact.New()
	r.SetSecrets([]string{"ab", "VALIDLONGSECRET123"}) // too-short first, real secret after
	out := r.Redact("x VALIDLONGSECRET123 y")

	if strings.Contains(out, "VALIDLONGSECRET123") {
		t.Errorf("a leading too-short value must not stop later secrets from registering: %q", out)
	}
}

// A duplicate value that is skipped must not stop the loop either. Kills the
// `continue` → `break` mutant on the dedup branch.
func TestSetSecrets_DuplicateDoesNotStopLaterSecrets(t *testing.T) {
	r := redact.New()
	r.SetSecrets([]string{"DUPLICATE_SECRET_1", "DUPLICATE_SECRET_1", "OTHER_SECRET_VALUE_2"})
	out := r.Redact("p DUPLICATE_SECRET_1 q OTHER_SECRET_VALUE_2 r")

	if strings.Contains(out, "OTHER_SECRET_VALUE_2") {
		t.Errorf("a duplicate value must not stop later secrets from registering: %q", out)
	}
	if strings.Contains(out, "DUPLICATE_SECRET_1") {
		t.Errorf("the de-duplicated secret must still be redacted: %q", out)
	}
}

// When one secret is a prefix of another, the set must be sorted longest-first so
// the longer secret is replaced as a whole — otherwise replacing the shorter one
// first leaves the longer secret's tail exposed. The inputs are given
// shortest-first so a no-op or reversed comparator produces a visible leak. Kills
// the sort-comparator no-op mutant and guards the documented longest-first invariant.
func TestSetSecrets_OverlappingRedactedLongestFirst(t *testing.T) {
	r := redact.New()
	const short = "ABCDEFGH"            // 8 chars
	const long = "ABCDEFGHIJKLMNOP"     // 16 chars; contains short as a prefix
	r.SetSecrets([]string{short, long}) // shortest-first on purpose
	out := r.Redact("v " + long + " w")

	if strings.Contains(out, "IJKLMNOP") {
		t.Errorf("overlapping secret leaked a tail (longest-first ordering broken): %q", out)
	}
	if strings.Contains(out, short) {
		t.Errorf("overlapping secret was not fully redacted: %q", out)
	}
}
