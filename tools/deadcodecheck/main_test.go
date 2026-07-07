// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"
)

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

func TestRunChecker_NoFindings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runChecker(&stdout, &stderr, nil, nil)
	if code != 0 {
		t.Errorf("runChecker = %d, want 0", code)
	}
	if stdout.String() != "OK: no dead code findings.\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "OK: no dead code findings.\n")
	}
}

func TestRunChecker_NoFindingsAllAllowed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := []byte("sdk/foo.go:1:1: unreachable func: Foo\nsdk/bar.go:2:2: unreachable func: Bar\n")
	code := runChecker(&stdout, &stderr, out, nil)
	if code != 0 {
		t.Errorf("runChecker = %d, want 0", code)
	}
	if stderr.Len() > 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
	if want := "OK: no unexpected dead code; 2 public SDK findings allowlisted.\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunChecker_AllAllowedMixed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := []byte("sdk/a.go:1:1: unreachable func: A\n")
	code := runChecker(&stdout, &stderr, out, nil)
	if code != 0 {
		t.Errorf("runChecker = %d, want 0", code)
	}
}

func TestRunChecker_UnexpectedExit1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out := []byte("kernel/foo.go:1:1: unreachable func: unused\n")
	code := runChecker(&stdout, &stderr, out, nil)
	if code != 1 {
		t.Errorf("runChecker = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Error("stderr should contain findings")
	}
}

// TestMain_AnalyzerError covers `main()` when the analyzer command fails.
func TestMain_AnalyzerError(t *testing.T) {
	savedExit := osExit
	savedCmd := newDeadcodeCmd
	t.Cleanup(func() { osExit = savedExit; newDeadcodeCmd = savedCmd })

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("osExit called") }
	newDeadcodeCmd = func() *exec.Cmd {
		return exec.Command("cmd", "/c", "echo", "error output")
	}

	func() {
		defer func() { recover() }()
		main()
	}()
	if exitCode != 1 {
		t.Errorf("osExit called with %d, want 1", exitCode)
	}
}

// TestMain_Success covers `main()` when the analyzer reports no findings.
func TestMain_Success(t *testing.T) {
	savedExit := osExit
	savedCmd := newDeadcodeCmd
	t.Cleanup(func() { osExit = savedExit; newDeadcodeCmd = savedCmd })

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("osExit called") }
	newDeadcodeCmd = func() *exec.Cmd {
		return exec.Command("cmd", "/c", "echo.")
	}

	func() {
		defer func() { recover() }()
		main()
	}()
	if exitCode != 0 {
		t.Errorf("osExit called with %d, want 0", exitCode)
	}
}

func TestRunChecker_AnalyzerFailedExit1(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runChecker(&stdout, &stderr, []byte("some output"), fmt.Errorf("exit status 1"))
	if code != 1 {
		t.Errorf("runChecker = %d, want 1", code)
	}
	if stderr.Len() == 0 {
		t.Error("stderr should contain error")
	}
}
