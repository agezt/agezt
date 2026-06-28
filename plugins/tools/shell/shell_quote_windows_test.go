// SPDX-License-Identifier: MIT

//go:build windows

package shell

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/warden"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestShell_QuotedCommandRunsVerbatim is the M958 regression guard: a perfectly
// valid Windows command that contains double-quotes (e.g. a quoted path) must
// run, not fail with "The filename, directory name, or volume label syntax is
// incorrect". Before the fix, os/exec's MSVC-style escaping of `cmd /C
// <command>` mangled any quoted command. Runs on the real host.
func TestShell_QuotedCommandRunsVerbatim(t *testing.T) {
	tool := NewWithWarden(warden.New(nil))
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

// TestShell_QuotedPathWithTrailingFlag pins the exact shape that flooded the
// live fleet journal before M958: a quoted path *followed by* a trailing switch,
// e.g. `dir "C:\some path" /b`. `cmd /S /C` strips only the first and last quote
// of the wrapped line, so a closing quote that is not the final character is the
// case most prone to mangling — and the one agents hit constantly while probing
// the host filesystem. Uses a real temp dir so the command must actually succeed.
func TestShell_QuotedPathWithTrailingFlag(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pending")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tool := NewWithWarden(warden.New(nil))
	for _, cmd := range []string{
		`dir "` + sub + `" /b`,        // quoted path + trailing flag (the fleet pattern)
		`cmd /C dir "` + sub + `" /b`, // explicit nested cmd /C, as agents often write
	} {
		in, _ := json.Marshal(shellInput{Command: cmd})
		res, err := tool.Invoke(context.Background(), in)
		if err != nil {
			t.Fatalf("%q: transport error: %v", cmd, err)
		}
		if res.IsError {
			t.Errorf("quoted-path-with-flag %q failed (cmd /C quoting bug?):\n%s", cmd, res.Output)
		}
	}
}
