// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the REASONING MODULE — the ratchet of W3.76v.
//
// The reasoning module owns the Council of Elders, the Conductor and the
// deep-research harness. The daemon used to hold THREE forwarders over the
// module's own operations — `Council`, `Conduct` and `Research` — a second
// spelling of every verb the contract's runner port already names, and a
// second copy of the module's whole surface for the council application, the
// research application and the three agent-facing tools to call through.
//
// The reasoning vocabulary is the CONTRACT's (`reasoningapi.Runner` — convene,
// conduct, research), so the module answers in those names and the daemon
// spells none of them.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the reasoning module's operations any more;
//   - the daemon still holds the module's WIRING (the ports it needs, the
//     setter that injects it) and the module's FACE.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"testing"
)

// reasoningForwardersInTheDaemon are the daemon's spellings of the reasoning
// module's operations.
var reasoningForwardersInTheDaemon = []string{
	"Council",
	"Conduct",
	"Research",
}

// TestTheDaemonDoesNotSpellOutTheReasoningModule pins the move: the reasoning
// capability is the module's, and one operation has one name.
func TestTheDaemonDoesNotSpellOutTheReasoningModule(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range reasoningForwardersInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the reasoning capability is the reasoning module's, and one operation has one name (W3.76v)", name)
		}
	}
}

// TestTheDaemonKeepsTheReasoningWiringAndTheFace pins what stayed: the host
// builds the module's PORTS (they are the host's own governed paths), the
// composition root INJECTS the module, and the daemon hands over the module's
// FACE — the one port the application and the tools bind.
func TestTheDaemonKeepsTheReasoningWiringAndTheFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"SetReasoning", "ReasoningDependencies", "ReasoningService"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the module's wiring and the module's face", name)
		}
	}
}
