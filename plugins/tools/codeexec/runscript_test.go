// SPDX-License-Identifier: MIT

package codeexec

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/warden"
)

// TestRunScript_LiveStdinContract proves the script-tool forge's execution
// contract (M794) against a real interpreter: the call's JSON input lands in
// ./stdin.txt, stdout comes back, and a non-zero exit reads as isError —
// exactly like a direct code_exec call. Skipped when no Python is installed.
func TestRunScript_LiveStdinContract(t *testing.T) {
	rt := DetectRuntimes()
	if _, ok := rt[LangPython]; !ok {
		t.Skip("python not installed")
	}
	tool := NewWithWarden(warden.New(nil), t.TempDir(), rt, true)

	out, isErr, err := tool.RunScript(context.Background(), LangPython,
		`import json; d=json.load(open("stdin.txt")); print("city=" + d["city"])`,
		`{"city":"izmir"}`)
	if err != nil {
		t.Fatalf("RunScript: %v", err)
	}
	if isErr {
		t.Fatalf("unexpected error result:\n%s", out)
	}
	if !strings.Contains(out, "city=izmir") {
		t.Fatalf("input did not reach the script via stdin.txt:\n%s", out)
	}

	// A failing script (non-zero exit) must read as a tool error, not a Go error.
	out, isErr, err = tool.RunScript(context.Background(), LangPython,
		`import sys; print("boom"); sys.exit(3)`, "{}")
	if err != nil {
		t.Fatalf("RunScript(fail): %v", err)
	}
	if !isErr || !strings.Contains(out, "exit code 3") {
		t.Fatalf("failure verdict wrong: isErr=%v out:\n%s", isErr, out)
	}

	// An unavailable language is an honest tool error too.
	out, isErr, err = tool.RunScript(context.Background(), "cobol", "x", "{}")
	if err != nil || !isErr || !strings.Contains(out, "not available") {
		t.Fatalf("unknown language: err=%v isErr=%v out=%s", err, isErr, out)
	}
}
