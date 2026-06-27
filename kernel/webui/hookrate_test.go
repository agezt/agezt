// SPDX-License-Identifier: MIT

package webui

import "testing"

// TestAllowHook is the regression guard for VULN-007: the token-free /hooks/ path
// must throttle a tight loop (max+burst per window) while leaving distinct
// workflow+source keys independent.
func TestAllowHook(t *testing.T) {
	s := &Server{}

	// First max+burst requests on one key pass; the next is refused.
	const key = "deploy|10.0.0.1"
	allowed := 0
	for i := 0; i < hookRatePerMin+hookRateBurst+5; i++ {
		if s.allowHook(key) {
			allowed++
		}
	}
	if allowed != hookRatePerMin+hookRateBurst {
		t.Errorf("allowed %d in one window, want %d", allowed, hookRatePerMin+hookRateBurst)
	}
	if s.allowHook(key) {
		t.Errorf("request beyond max+burst should be refused")
	}

	// A different source (or different workflow) has its own independent budget.
	if !s.allowHook("deploy|10.0.0.2") {
		t.Errorf("a distinct source key should not be throttled by another's usage")
	}
	if !s.allowHook("other|10.0.0.1") {
		t.Errorf("a distinct workflow key should not be throttled by another's usage")
	}
}
