// SPDX-License-Identifier: MIT

package edict

import "testing"

// stripPunctAdjacentWhitespace documents that it removes a space bordering
// punctuation on *either* side. The fork-bomb evasion tests exercise it only
// indirectly, and every optional space in `:(){ :|:& };:` happens to have
// punctuation on its right, so the forward (right-side) scan alone normalizes it
// — the backward (left-side) scan is never exercised. Mutation testing (M492)
// confirmed that: disabling the backward loop (`j >= 0` → a never-true guard)
// survived. A broken left-side scan would let a spacing variant whose only
// punctuation neighbour is on the left slip past floor-rule normalization, so the
// contract is pinned directly here.
func TestStripPunctAdjacentWhitespace(t *testing.T) {
	cases := []struct{ name, in, want string }{
		// Punctuation on the LEFT only — only the backward scan can strip this.
		{"left-punct", "x} y", "x}y"},
		// Punctuation on the RIGHT only — the forward scan.
		{"right-punct", "x {y", "x{y"},
		// Two alphanumeric words — the space must be preserved, never merged
		// (merging is the M426 false-hard-deny regression).
		{"word-word", "foo bar", "foo bar"},
		// Trailing space with no punctuation: preserved, and the forward scan must
		// not read past the end (guards the `j < len(rs)` bound).
		{"trailing-space", "ab ", "ab "},
		// The canonical fork bomb the floor rule targets, normalized to its
		// no-space form.
		{"fork-bomb", ":(){ :|:& };:", ":(){:|:&};:"},
	}
	for _, c := range cases {
		if got := stripPunctAdjacentWhitespace(c.in); got != c.want {
			t.Errorf("%s: stripPunctAdjacentWhitespace(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
