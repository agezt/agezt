// SPDX-License-Identifier: MIT

package channel

import (
	"strconv"
	"sync"
	"testing"
)

// TestRegistryConcurrentAccess is the regression guard for VULN-002: the
// process-global registry/live/liveInstances maps are read (control-plane
// /api/channel/list, auto-polled by the web UI on load) concurrently with
// boot-time writes (RegisterAll, SetLive, SetLiveInstances). Before the mutex,
// this raced a map read against a write — a fatal, unrecoverable Go runtime abort
// (remote-triggerable crash DoS). Run with `-race` to assert it is synchronised.
func TestRegistryConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	const n = 50

	// Writers: register manifests + flip live sets, as the daemon does during boot.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			k := "ch" + strconv.Itoa(i)
			RegisterManifest(Manifest{Kind: k, Display: k})
			SetLive([]string{k})
			SetLiveInstances([]string{InstanceKey(k, "a")})
		}
	}()

	// Readers: the accessors the control-plane handlers call per request.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				_ = Manifests()
				_, _ = LookupManifest("ch" + strconv.Itoa(i))
				_ = IsLive("ch" + strconv.Itoa(i))
				_ = IsLiveInstance(InstanceKey("ch"+strconv.Itoa(i), "a"))
			}
		}()
	}

	wg.Wait()
}
