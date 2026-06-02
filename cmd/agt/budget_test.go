// SPDX-License-Identifier: MIT

package main

import "testing"

func TestUsdToMicrocents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"0.01", 10_000_000, true}, // $1 = 1e9 mc → $0.01 = 1e7
		{"1", 1_000_000_000, true},
		{"1.5", 1_500_000_000, true},
		{"$0.25", 250_000_000, true}, // leading $ tolerated
		{"  0.5 ", 500_000_000, true},
		{"0", 0, true},
		{"-1", 0, false},  // negative rejected
		{"abc", 0, false}, // non-numeric rejected
		{"", 0, false},    // empty rejected
	}
	for _, c := range cases {
		got, err := usdToMicrocents(c.in)
		if c.ok && err != nil {
			t.Errorf("usdToMicrocents(%q) unexpected err: %v", c.in, err)
			continue
		}
		if !c.ok {
			if err == nil {
				t.Errorf("usdToMicrocents(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if got != c.want {
			t.Errorf("usdToMicrocents(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// fmtUSD ∘ usdToMicrocents round-trips at 4-decimal precision.
func TestUsdMicrocentsRoundTrip(t *testing.T) {
	mc, err := usdToMicrocents("0.0500")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmtUSD(mc); got != "$0.0500" {
		t.Errorf("round-trip = %q want $0.0500", got)
	}
}
