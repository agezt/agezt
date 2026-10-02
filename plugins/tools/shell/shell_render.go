// SPDX-License-Identifier: MIT

package shell

// Provenance: Package shell: renderResult (combined stdout/stderr, truncation,
//             exit-code propagation) + ShellHint + resolveShell (shell binary
//             resolution by GOOS). Extracted from shell.go during the Day-211
//             god-file split. Public API unchanged.

import (
	"fmt"
	"runtime"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/warden"
)

func renderResult(timeout time.Duration, res *warden.Result) toolapi.Result {
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

	// Build the FINAL, model-facing line FIRST — status prefix and exit-code
	// suffix included — and only then apply the budget to it.
	//
	// This ordering matters and used to be wrong. The budget was enforced on
	// `combined`, and the status line was prepended afterwards, so a command
	// that both overflowed the budget AND timed out shipped
	// len(MaxOutputBytes) + len("timed out after 30s\n") bytes to the model.
	// The documented invariant is on Result.Output, and a budget the model can
	// exceed is not a budget. It also made
	// TestInvoke_RealWarden_CombinedBudgetHeld machine-speed dependent: kill the
	// command before 30s and no prefix is added and the test passes; let it run
	// long enough to overflow and it fails — same code, two verdicts.
	prefix, suffix, isError := "", "", false
	if res.TimedOut {
		prefix, isError = fmt.Sprintf("timed out after %s\n", timeout), true
	} else if res.ExitCode != 0 {
		suffix, isError = fmt.Sprintf("\n[exit code %d]", res.ExitCode), true
	}

	final := make([]byte, 0, len(prefix)+len(combined)+len(suffix))
	final = append(final, prefix...)
	final = append(final, combined...)
	final = append(final, suffix...)

	if res.Truncated || len(final) > MaxOutputBytes {
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
		if len(final) > keep {
			// Keep the tail, but never eat the status prefix: a truncated
			// result that lost "timed out after 30s" would read as a command
			// that simply produced a lot of output.
			if len(prefix) < keep {
				tail := final[len(final)-keep:]
				tail = tail[len(prefix):] // drop the prefix from the tail copy
				final = append(append([]byte{}, prefix...), tail...)
			} else {
				final = final[len(final)-keep:]
			}
		}
		final = append([]byte(marker), final...)
	}

	return toolapi.Result{Output: string(final), IsError: isError}
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
