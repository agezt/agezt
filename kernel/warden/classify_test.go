// SPDX-License-Identifier: MIT

package warden

import (
	"errors"
	"testing"
)

// TestClassifyWaitErr pins M475: a non-*exec.ExitError engine failure must be
// surfaced even when it coincides with a non-zero exit code (the old code's
// ExitCode==0 guard swallowed exactly those). nil and timed-out runs are absorbed.
func TestClassifyWaitErr(t *testing.T) {
	if err := classifyWaitErr(nil, false, "x"); err != nil {
		t.Errorf("nil wait error must classify as no engine error, got %v", err)
	}
	if err := classifyWaitErr(errors.New("killed"), true, "x"); err != nil {
		t.Errorf("a timed-out run must be absorbed (reported via TimedOut), got %v", err)
	}
	// The fix: a genuine non-ExitError failure (e.g. WaitDelay abandonment, I/O
	// error) must be returned, not swallowed.
	if err := classifyWaitErr(errors.New("io: file already closed"), false, "x"); err == nil {
		t.Error("a non-ExitError engine failure must be surfaced, got nil (swallowed)")
	}
}
