// SPDX-License-Identifier: MIT

// Package shell: renderResult (combined stdout/stderr, truncation, exit-code
// propagation) + ShellHint + resolveShell (shell binary resolution by GOOS).
// Extracted from shell.go during the Day-211 god-file split. Public API
// unchanged.
package shell


import (
	"fmt"
	"runtime"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/warden"
)
func renderResult(timeout time.Duration, res *warden.Result) agent.Result {
	// Combine streams the way the previous implementation did
	// (CombinedOutput). Stderr appended after stdout keeps the order
	// stable across shells.
	combined := append([]byte{}, res.Stdout...)
	if len(res.Stderr) > 0 {
		if len(combined) > 0 {
			combined = append(combined, '\n')
		}
		combined = append(combined, res.Stderr...)
	}
	if res.Truncated || len(combined) > MaxOutputBytes {
		// Enforce the model-facing budget on the COMBINED output. Warden wires
		// one capBuffer per stream, so stdout and stderr are EACH allowed
		// MaxOutputBytes and the concatenation can reach twice the budget —
		// and when neither stream alone trips its cap, res.Truncated stays
		// false and the overflow would ship with no marker at all.
		// Tail-truncate (a failing command's final lines matter most) and
		// always mark, marker included, so the total stays within budget.
		const marker = "[truncated to last 64 KiB]\n"
		keep := MaxOutputBytes - len(marker)
		if keep < 0 {
			keep = 0
		}
		if len(combined) > keep {
			combined = combined[len(combined)-keep:]
		}
		combined = append([]byte(marker), combined...)
	}

	if res.TimedOut {
		return agent.Result{
			Output:  fmt.Sprintf("timed out after %s\n%s", timeout, combined),
			IsError: true,
		}
	}
	if res.ExitCode != 0 {
		return agent.Result{
			Output:  fmt.Sprintf("%s\n[exit code %d]", combined, res.ExitCode),
			IsError: true,
		}
	}
	return agent.Result{Output: string(combined)}
}
func (t *Tool) ShellHint() (string, string) { return t.resolveShell() }
func (t *Tool) resolveShell() (string, string) {
	if t.Shell != "" {
		arg := t.ShellArg
		if arg == "" {
			arg = "-c"
		}
		return t.Shell, arg
	}
	if runtime.GOOS == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}
