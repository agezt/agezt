// SPDX-License-Identifier: MIT

package main

import "testing"

// TestFailedByReasonStr pins the M36 breakdown formatter: known reasons in
// a fixed order, unknowns sorted after, zero/empty → "".
func TestFailedByReasonStr(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"nil", nil, ""},
		{"empty", map[string]any{}, ""},
		{"single", map[string]any{"timeout": float64(3)}, "timeout=3"},
		{
			"fixed-order",
			map[string]any{"canceled": float64(1), "error": float64(2), "timeout": float64(4)},
			"error=2, timeout=4, canceled=1",
		},
		{
			"unknown-tag-sorts-after-known",
			map[string]any{"error": float64(1), "zztop": float64(5), "aaa": float64(2)},
			"error=1, aaa=2, zztop=5",
		},
		{"zero-counts-dropped", map[string]any{"timeout": float64(0), "error": float64(1)}, "error=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := failedByReasonStr(c.in); got != c.want {
				t.Errorf("failedByReasonStr(%v) = %q want %q", c.in, got, c.want)
			}
		})
	}
}
