// SPDX-License-Identifier: MIT

package main

import "testing"

func TestFindingLines(t *testing.T) {
	out := []byte("\n sdk\\approvals.go:35:18: unreachable func: Client.Approve \n\nkernel/foo.go:1:1: unreachable func: unused\n")

	got := findingLines(out)
	want := []string{
		`sdk\approvals.go:35:18: unreachable func: Client.Approve`,
		"kernel/foo.go:1:1: unreachable func: unused",
	}
	if len(got) != len(want) {
		t.Fatalf("findingLines returned %d lines, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("findingLines[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestIsAllowedSDKFinding(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "windows sdk path",
			line: `sdk\approvals.go:35:18: unreachable func: Client.Approve`,
			want: true,
		},
		{
			name: "unix sdk path",
			line: "sdk/mailbox.go:58:18: unreachable func: Client.SendMail",
			want: true,
		},
		{
			name: "non sdk finding",
			line: "kernel/agent/foo.go:1:1: unreachable func: unused",
			want: false,
		},
		{
			name: "sdk non finding output",
			line: "sdk/mailbox.go: analyzer failed",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAllowedSDKFinding(tt.line); got != tt.want {
				t.Fatalf("isAllowedSDKFinding(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
