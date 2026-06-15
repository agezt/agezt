// SPDX-License-Identifier: MIT

//go:build windows

package shell

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestShell_QuotedCommandRunsVerbatim is the M958 regression guard: a perfectly
// valid Windows command that contains double-quotes (e.g. a quoted path) must
// run, not fail with "The filename, directory name, or volume label syntax is
// incorrect". Before the fix, os/exec's MSVC-style escaping of `cmd /C
// <command>` mangled any quoted command. Runs on the real host.
func TestShell_QuotedCommandRunsVerbatim(t *testing.T) {
	tool := New()
	for _, cmd := range []string{
		`dir "."`,            // quoted path argument
		`echo "hello world"`, // quoted literal with a space
	} {
		in, _ := json.Marshal(shellInput{Command: cmd})
		res, err := tool.Invoke(context.Background(), in)
		if err != nil {
			t.Fatalf("%q: transport error: %v", cmd, err)
		}
		if res.IsError {
			t.Errorf("quoted command %q failed (cmd /C quoting bug?):\n%s", cmd, res.Output)
		}
	}
	// Sanity: the quoted echo's text actually came through.
	in, _ := json.Marshal(shellInput{Command: `echo "hello world"`})
	res, _ := tool.Invoke(context.Background(), in)
	if !strings.Contains(res.Output, "hello world") {
		t.Errorf("quoted echo output missing the text: %q", res.Output)
	}
}
