// SPDX-License-Identifier: MIT

package shell

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
)

// TestShell_ResolvesExternalProgram is the regression guard for M957: the shell
// tool must run with a working PATH so external programs resolve. Before the fix
// the tool passed no Env → warden gave the child an EMPTY environment → on
// Windows cmd.exe could find nothing ("'where' is not recognized") and even
// built-ins failed ("syntax is incorrect"). This runs a PATH-dependent command
// on the real host and asserts it succeeds.
func TestShell_ResolvesExternalProgram(t *testing.T) {
	// A command that requires resolving an EXTERNAL program via PATH.
	cmd := "ls /" // unix: ls lives on PATH
	if runtime.GOOS == "windows" {
		cmd = "where where" // where.exe lives in System32, found via PATH
	}

	tool := New()
	in, _ := json.Marshal(shellInput{Command: cmd})
	res, err := tool.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke returned a transport error: %v", err)
	}
	if res.IsError {
		t.Fatalf("PATH-dependent command %q failed — shell env has no PATH?\n%s", cmd, res.Output)
	}
	if res.Output == "" {
		t.Errorf("expected output from %q, got empty", cmd)
	}
}
