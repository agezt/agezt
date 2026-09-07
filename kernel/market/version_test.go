// SPDX-License-Identifier: MIT

package market

import "testing"

// The semver precedence table. The chain in the first test is the spec's own
// example set — every adjacent pair must order low-to-high.
func TestCompareVersionsSemverChain(t *testing.T) {
	chain := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-beta.2",
		"1.0.0-beta.11",
		"1.0.0-rc.1",
		"1.0.0",
	}
	for i := 0; i+1 < len(chain); i++ {
		if got := CompareVersions(chain[i], chain[i+1]); got != -1 {
			t.Errorf("CompareVersions(%q, %q) = %d, want -1", chain[i], chain[i+1], got)
		}
		// And antisymmetric, so ordering is not an accident of argument order.
		if got := CompareVersions(chain[i+1], chain[i]); got != 1 {
			t.Errorf("CompareVersions(%q, %q) = %d, want 1", chain[i+1], chain[i], got)
		}
	}
}

func TestCompareVersionsCores(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
		why  string
	}{
		{"1.9.0", "1.10.0", -1, "multi-digit minor is bigger than nine, not smaller"},
		{"1.0.10", "1.0.9", 1, "multi-digit patch"},
		{"2.0.0", "1.99.99", 1, "major dominates"},
		{"2.0.0", "2.0.0", 0, "identical"},
		{"1.0.0", "1.0.0+build.7", 0, "build metadata is ignored by precedence"},
		{"1.0.0-2", "1.0.0-10", -1, "numeric identifiers compare numerically"},
		{"1.0.0-1", "1.0.0-alpha", -1, "numeric identifier ranks below alphanumeric"},
	} {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d (%s)", tc.a, tc.b, got, tc.want, tc.why)
		}
	}
}

// Versions reach the comparator from installed.json, which an operator can
// hand-edit. Malformed input must yield a deterministic answer, never a panic
// and never a parse error that breaks browsing.
func TestCompareVersionsMalformedNeverPanics(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"garbage", "1.0.0"},
		{"1.2", "1.2"},
		{"", "1.0.0"},
		{"1.0.0-", "1.0.0"},
		{"1.a.0", "1.b.0"},
		{"1.0.0.0", "1.0.0"},
	} {
		got := CompareVersions(tc.a, tc.b) // must not panic
		if got < -1 || got > 1 {
			t.Errorf("CompareVersions(%q, %q) = %d, out of range", tc.a, tc.b, got)
		}
	}
}
