// SPDX-License-Identifier: MIT

package sdk

import "testing"

// TestIntFromAny covers every branch of intFromAny. Over the wire, JSON numbers
// always decode to float64 (the only case the parse-path tests reach), but the
// helper also accepts int64 and int for direct/programmatic use — plus a
// default for anything non-numeric. We call it directly to exercise the int64
// and int cases the JSON path can't produce.
func TestIntFromAny(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
	}{
		{"float64", float64(7), 7},
		{"int64", int64(9), 9},
		{"int", 11, 11},
		{"nil", nil, 0},
		{"string is non-numeric", "not a number", 0},
	}
	for _, c := range cases {
		if got := intFromAny(c.in); got != c.want {
			t.Errorf("intFromAny(%s=%v) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}
