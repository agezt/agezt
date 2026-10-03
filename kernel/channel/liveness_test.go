// SPDX-License-Identifier: MIT

package channel

import "testing"

// A channel that dies is no longer reported live — including one that died
// before the daemon recorded the live set (a port already in use fails within
// milliseconds of boot, before SetLiveInstances runs).
func TestMarkInstanceDead(t *testing.T) {
	// Package-global state: use kinds no other test touches.
	MarkInstanceDead("zzdead#early") // dies before the live set is recorded
	SetLive([]string{"zzdead", "zzalive"})
	SetLiveInstances([]string{"zzdead", "zzdead#early", "zzalive"})
	t.Cleanup(func() {
		SetLive(nil)
		SetLiveInstances(nil)
		// The dead set is deliberately never reset in production; clear this
		// test's kinds so a -count=N rerun starts from scratch.
		mu.Lock()
		delete(deadInstances, "zzdead")
		delete(deadInstances, "zzdead#early")
		mu.Unlock()
	})

	if IsLiveInstance("zzdead#early") {
		t.Error("an instance that died before SetLiveInstances was resurrected by it")
	}
	if !IsLiveInstance("zzdead") || !IsLive("zzdead") {
		t.Error("the kind's surviving default instance must still be live")
	}

	MarkInstanceDead("zzdead")
	if IsLiveInstance("zzdead") {
		t.Error("a dead default instance is still reported live")
	}
	if IsLive("zzdead") {
		t.Error("a kind whose every instance died is still reported live")
	}
	if !IsLive("zzalive") || !IsLiveInstance("zzalive") {
		t.Error("an unrelated kind was affected")
	}
}
