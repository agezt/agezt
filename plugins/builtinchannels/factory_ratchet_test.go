// SPDX-License-Identifier: MIT

package builtinchannels

import (
	"testing"

	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/channelwire"
)

// factoryTODO is the RATCHET for the channel-factory migration (Phase 2.1 of
// docs/REFACTORING-SCAN-2026-08.md). The migration is COMPLETE — every
// manifest kind registers a channelwire.Factory — so the set is empty; it
// stays (with the test below) as the permanent drift alarm: a manifest kind
// in neither the factory registry nor this set fails loudly (the 34-vs-27
// silent-drift class this migration existed to close — a channel shipped
// with a manifest but no wiring).
var factoryTODO = map[string]bool{}

// TestEveryManifestHasFactoryOrTODO is the drift alarm: every registered
// manifest kind must either have a channelwire factory or be an acknowledged
// not-yet-migrated entry above — and never both.
func TestEveryManifestHasFactoryOrTODO(t *testing.T) {
	RegisterAll()
	for _, m := range channel.Manifests() {
		_, hasFactory := channelwire.Lookup(m.Kind)
		inTODO := factoryTODO[m.Kind]
		switch {
		case hasFactory && inTODO:
			t.Errorf("%s migrated to a channelwire factory — delete it from factoryTODO to lock in the progress", m.Kind)
		case !hasFactory && !inTODO:
			t.Errorf("%s has a manifest but neither a channelwire factory nor a factoryTODO entry — a channel shipped without wiring (the allInsts drift class); register a Factory for it", m.Kind)
		}
	}
	// The inverse: a TODO entry whose manifest vanished is stale bookkeeping.
	for kind := range factoryTODO {
		if _, ok := channel.LookupManifest(kind); !ok {
			t.Errorf("factoryTODO lists %s but no such manifest is registered — remove the stale entry", kind)
		}
	}
}
