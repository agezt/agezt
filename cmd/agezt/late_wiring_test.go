// SPDX-License-Identifier: MIT

package main

import "testing"

// TestTheLateHolderAnswersNilUntilTheRootPublishesIt: the root publishes the
// late artifacts, and the adapter asks.
//
// The daemon's pulse engine, live channels and standing runner come up AFTER
// the adapter is built. The root hands the adapter a way to ASK (the holder),
// and the answer is nil until the artifact is there — the adapter has no
// phase to bind and no setter to push through.
func TestTheLateHolderAnswersNilUntilTheRootPublishesIt(t *testing.T) {
	var holder lateHolder[int]
	if got := holder.get(); got != 0 {
		t.Errorf("the holder answered %d before anything was published, want the zero value", got)
	}
	holder.set(42)
	if got := holder.get(); got != 42 {
		t.Errorf("the holder answered %d, want the published 42", got)
	}
	holder.set(0)
	if got := holder.get(); got != 0 {
		t.Errorf("the holder answered %d, want the zero value again", got)
	}
}
