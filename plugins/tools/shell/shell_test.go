// SPDX-License-Identifier: MIT

package shell

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShell_RunsCommand(t *testing.T) {
	sh := New()
	cmd := "echo hello"
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	}
	in, _ := json.Marshal(shellInput{Command: cmd})
	r, err := sh.Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Errorf("unexpected IsError; output=%s", r.Output)
	}
	if !strings.Contains(r.Output, "hello") {
		t.Errorf("output missing 'hello': %q", r.Output)
	}
}

func TestShell_MissingCommand_IsErrorNotFatal(t *testing.T) {
	r, err := New().Invoke(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError {
		t.Errorf("expected IsError for missing command; output=%s", r.Output)
	}
}

func TestShell_NonzeroExit_FlaggedNotPanicked(t *testing.T) {
	cmd := "exit 7"
	if runtime.GOOS == "windows" {
		cmd = "exit 7"
	}
	in, _ := json.Marshal(shellInput{Command: cmd})
	r, err := New().Invoke(context.Background(), in)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError {
		t.Errorf("non-zero exit should set IsError")
	}
}

func TestShell_Timeout(t *testing.T) {
	// Pick a command that the wrapper shell runs directly (no fork) so
	// the ctx cancellation actually halts within WaitDelay even on Windows
	// where cmd /C can't reap child processes. A busy-loop in cmd itself
	// is reliable.
	var cmd string
	if runtime.GOOS == "windows" {
		// Endless until cmd dies; no child process.
		cmd = "for /L %i in (1,0,2) do @ver >NUL"
	} else {
		// Pure bash loop; no fork.
		cmd = "while :; do :; done"
	}
	in, _ := json.Marshal(shellInput{Command: cmd, TimeoutMS: 150})
	start := time.Now()
	r, err := New().Invoke(context.Background(), in)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !r.IsError {
		t.Errorf("expected timeout to set IsError; output=%s", r.Output)
	}
	if !strings.Contains(r.Output, "timed out") {
		t.Errorf("output missing 'timed out': %q", r.Output)
	}
	// Allow generous slack for ctx propagation + WaitDelay (500ms).
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %s", elapsed)
	}
}

func TestShell_BadJSONInput(t *testing.T) {
	_, err := New().Invoke(context.Background(), json.RawMessage(`{bogus`))
	if err == nil {
		t.Errorf("expected error for malformed input JSON")
	}
}

func TestShell_Definition(t *testing.T) {
	def := New().Definition()
	if def.Name != "shell" {
		t.Errorf("Name=%q want shell", def.Name)
	}
	if !strings.Contains(string(def.InputSchema), `"command"`) {
		t.Errorf("schema missing 'command' field")
	}
}
