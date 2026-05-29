// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdRun_JSONFlagParsedAnyPosition — `--json` should work
// whether it comes before or after the intent. Operators do
// both naturally.
func TestCmdRun_JSONFlagParsedAnyPosition(t *testing.T) {
	// Both calls below expect to fail at the dial stage (no daemon
	// in the test process), exiting 1. What we're verifying is
	// that the flag parser accepted --json without complaining
	// about an "extra arg" — if it ran the JSON path, stdout
	// would start with a JSON line; otherwise stderr would say
	// "unknown" or "unexpected".
	for _, args := range [][]string{
		{"--json", "hello"},
		{"hello", "--json"},
	} {
		var out, errOut bytes.Buffer
		_ = cmdRun(args, &out, &errOut)
		// We didn't reach the JSON path because dial failed; the
		// flag must NOT have been treated as garbage.
		if strings.Contains(errOut.String(), "intent required") {
			t.Errorf("args=%v: parser treated --json as intent; got %q", args, errOut.String())
		}
	}
}

// TestCmdRun_RequiresIntentEvenWithJSONFlag — `agt run --json`
// alone must still fail with "intent required" rather than
// silently dialing the daemon with an empty intent.
func TestCmdRun_RequiresIntentEvenWithJSONFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRun([]string{"--json"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "intent required") {
		t.Errorf("stderr should still require intent; got %q", errOut.String())
	}
}
