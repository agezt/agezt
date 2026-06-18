// SPDX-License-Identifier: MIT

package main

import "testing"

func TestScheduleStatusLine(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{
			name:  "no schedules is quiet",
			input: map[string]any{"total": float64(0), "enabled": float64(0), "resident": true},
			want:  "",
		},
		{
			name:  "enabled schedules",
			input: map[string]any{"total": float64(3), "enabled": float64(2), "running": float64(0), "resident": true},
			want:  "3 (2 enabled)",
		},
		{
			name:  "running schedules",
			input: map[string]any{"total": float64(3), "enabled": float64(2), "running": float64(1), "resident": true},
			want:  "3 (2 enabled, 1 running)",
		},
		{
			name:  "enabled but resident offline",
			input: map[string]any{"total": float64(3), "enabled": float64(2), "running": float64(0), "resident": false},
			want:  "3 (2 enabled, resident offline)",
		},
		{
			name:  "running but resident offline is visible",
			input: map[string]any{"total": float64(3), "enabled": float64(2), "running": float64(1), "resident": false},
			want:  "3 (2 enabled, 1 running, resident offline)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduleStatusLine(tc.input); got != tc.want {
				t.Fatalf("scheduleStatusLine() = %q, want %q", got, tc.want)
			}
		})
	}
}
