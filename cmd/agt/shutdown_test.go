// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdShutdown_HelpExitsCleanly — help path mustn't try to dial
// the daemon; pure flag parsing.
func TestCmdShutdown_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdShutdown([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"shutdown", "exit gracefully"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdShutdown_RejectsUnknownFlag — the parser must catch typos
// rather than silently no-op. Pre-dial, so no daemon required.
func TestCmdShutdown_RejectsUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdShutdown([]string{"--force"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should flag unknown arg; got %q", errOut.String())
	}
}
