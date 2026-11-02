// SPDX-License-Identifier: MIT

package runtime

// Provenance: the daemon's face over the AGENTS MODULE's roster service
//             (W3.76n).
//
// The roster's WRITES are the module's: the module keeps the profile
// lifecycle, the events those writes publish and the rule that a shipped
// guardian may be paused or retired but never removed. The daemon keeps the
// names — AddProfile, UpdateProfile, RemoveProfile, SetProfileEnabled and
// SetProfileRetired are still the daemon's verbs — and forwards.
//
// The accessor lives HERE, beside the module's face, and not in
// accessors_roster.go: that file holds the roster's PUBLIC verbs, and the
// control plane's guard reads it to learn which of them the application layer
// owns. A private accessor is not a verb the application layer guards.

import (
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	agentsapi "github.com/agezt/agezt/kernel/modules/agents/api"
)

// rosterService returns the injected roster service, or nil.
//
// The service is the AGENTS MODULE's: the module keeps the lifecycle, the
// daemon keeps the names. The composition root hands the BUILDER over (see
// runtime.Config.RosterService) and the kernel calls it while opening itself
// — so the daemon never needs a setter for it.
func (k *Kernel) rosterService() agentsapi.RosterService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.rosterSvc
}

// rosterPublisher is the host's bus seen from the roster: the module
// publishes one event and the host drops the reply.
//
// The bus is the HOST's; the roster only says what happened to an agent, so
// the daemon hands over a way to ASK it — the same shape the MCP registry
// uses (W3.71).
type rosterPublisher struct{ b *bus.Bus }

// Publish emits one event on the host's bus, when the host has one.
func (p rosterPublisher) Publish(spec event.Spec) {
	if p.b == nil {
		return
	}
	_, _ = p.b.Publish(spec)
}
