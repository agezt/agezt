// SPDX-License-Identifier: MIT

package runtime

import (
	"errors"
	"testing"
)

// TestCloseAll_ClosesAllDespiteError pins M477: closeAll must invoke every closer
// even when an earlier one errors (the old Close short-circuited and leaked the
// remaining handles, notably the journal fd), and must report the failure.
func TestCloseAll_ClosesAllDespiteError(t *testing.T) {
	var calls int
	boom := errors.New("boom")
	mk := func(err error) func() error {
		return func() error { calls++; return err }
	}

	err := closeAll(mk(nil), mk(boom), mk(nil), mk(nil), mk(boom))
	if calls != 5 {
		t.Errorf("closeAll invoked %d closers, want 5 (later closers leaked)", calls)
	}
	if err == nil {
		t.Fatal("closeAll must report the close errors")
	}
	if !errors.Is(err, boom) {
		t.Errorf("joined error must contain the underlying failure, got %v", err)
	}
}
