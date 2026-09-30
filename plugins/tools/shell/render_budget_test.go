// SPDX-License-Identifier: MIT

package shell

import (
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/warden"
)

// TestRenderResult_RespectsBudgetOnFinalOutput pins the budget to the string the
// MODEL actually sees — Result.Output — across every shape renderResult can
// produce, including the status prefix and exit-code suffix.
//
// This is the deterministic replacement for what
// TestInvoke_RealWarden_CombinedBudgetHeld was reaching for indirectly. That
// test drives a real cmd.exe through the real Warden and depends on the command
// running long enough to overflow the budget, so its verdict flipped on machine
// speed: kill it before DefaultTimeout and no status prefix is added and the
// test passes; let it overflow and it failed.
//
// The bug it eventually caught was real. The budget was applied to the
// concatenated streams, and the "timed out after 30s\n" prefix was prepended
// afterwards, so a command that both overflowed AND timed out shipped
// MaxOutputBytes + len(prefix) bytes. The documented invariant is on
// Result.Output, and a budget the model can exceed is not a budget.
func TestRenderResult_RespectsBudgetOnFinalOutput(t *testing.T) {
	big := strings.Repeat("Z", 200000)

	cases := []struct {
		name          string
		res           *warden.Result
		wantMarker    bool
		wantIsError   bool
		wantHeadHas   string // substring that must survive truncation
		wantTailIsErr string
	}{
		{
			name:        "clean output over budget",
			res:         &warden.Result{Stdout: []byte(big)},
			wantMarker:  true,
			wantHeadHas: "[truncated to last 64 KiB]",
		},
		{
			name:        "both streams over budget",
			res:         &warden.Result{Stdout: []byte(big), Stderr: []byte(big)},
			wantMarker:  true,
			wantHeadHas: "[truncated to last 64 KiB]",
		},
		{
			name:        "warden reported truncated",
			res:         &warden.Result{Stdout: []byte(big), Truncated: true},
			wantMarker:  true,
			wantHeadHas: "[truncated to last 64 KiB]",
		},
		{
			name:        "timed out and over budget",
			res:         &warden.Result{Stdout: []byte(big), TimedOut: true},
			wantMarker:  true,
			wantIsError: true,
			// The status prefix must not be eaten by the tail truncation: a
			// result that lost "timed out after …" would read as a command
			// that simply produced a lot of output.
			wantHeadHas: "timed out after 30s",
		},
		{
			name:        "nonzero exit and over budget",
			res:         &warden.Result{Stdout: []byte(big), ExitCode: 3},
			wantMarker:  true,
			wantIsError: true,
			wantHeadHas: "[truncated to last 64 KiB]",
		},
		{
			name:        "timed out, nonzero exit and both streams over budget",
			res:         &warden.Result{Stdout: []byte(big), Stderr: []byte(big), TimedOut: true, ExitCode: 2},
			wantMarker:  true,
			wantIsError: true,
			wantHeadHas: "timed out after 30s",
		},
		{
			name:        "small output, timed out",
			res:         &warden.Result{Stdout: []byte("hi"), TimedOut: true},
			wantIsError: true,
			wantHeadHas: "timed out after 30s",
		},
		{
			name:        "small output, nonzero exit",
			res:         &warden.Result{Stdout: []byte("hi"), ExitCode: 1},
			wantIsError: true,
			wantHeadHas: "[exit code 1]",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := renderResult(30*time.Second, c.res)

			if n := len(out.Output); n > MaxOutputBytes {
				t.Fatalf("Output is %d bytes, want <= MaxOutputBytes (%d) — over by %d",
					n, MaxOutputBytes, n-MaxOutputBytes)
			}
			if got := strings.Contains(out.Output, "[truncated"); got != c.wantMarker {
				t.Errorf("truncation marker present = %v, want %v", got, c.wantMarker)
			}
			if out.IsError != c.wantIsError {
				t.Errorf("IsError = %v, want %v", out.IsError, c.wantIsError)
			}
			if c.wantHeadHas != "" && !strings.Contains(out.Output, c.wantHeadHas) {
				t.Errorf("output lost %q, which must survive truncation", c.wantHeadHas)
			}
		})
	}
}
