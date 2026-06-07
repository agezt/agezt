// SPDX-License-Identifier: MIT

package approval

import (
	"testing"
	"time"
)

// New must default an unset (zero) Config.Timeout to DefaultTimeout: a Registry
// built from a bare Config{} would otherwise carry a 0 timeout, which makes every
// submitted approval auto-deny instantly (timeout fires at once). The end-to-end
// tests all pass an explicit Timeout, so the default path was unpinned — mutation
// testing (M504) showed `if timeout <= 0` could weaken to `< 0`, leaving the zero
// value un-defaulted, undetected. White-box so it can read the resolved field
// directly and deterministically (no real-timer race).
func TestNew_DefaultsUnsetTimeout(t *testing.T) {
	if got := New(Config{}).timeout; got != DefaultTimeout {
		t.Errorf("New with unset Timeout: timeout = %v, want DefaultTimeout %v (a 0 timeout auto-denies every approval instantly)", got, DefaultTimeout)
	}
	// A negative value is also treated as "unset".
	if got := New(Config{Timeout: -1}).timeout; got != DefaultTimeout {
		t.Errorf("New with negative Timeout: timeout = %v, want DefaultTimeout %v", got, DefaultTimeout)
	}
	// An explicit positive timeout is honored verbatim.
	if got := New(Config{Timeout: 2 * time.Second}).timeout; got != 2*time.Second {
		t.Errorf("explicit Timeout not honored: %v", got)
	}
}
