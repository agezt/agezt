// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the SCRIPT-TOOL FORGE — the ratchet of W3.76t.
//
// The forge is the EXTENSIONS MODULE's: the module keeps the catalogue's
// rules, the lifecycle and the bridge that folds the promoted scripts into a
// run's tool map. The daemon used to hold SEVEN forwarders over the module's
// own operations (DraftScriptTool, UpdateScriptTool, TestScriptTool,
// PromoteScriptTool, RequestToolPromotion, QuarantineScriptTool and
// RemoveScriptTool) — a second spelling of every forge verb, and a second copy
// of the whole forge surface for the application layer and the plugin to call
// through.
//
// The forge's vocabulary is the CONTRACT's (kernel/contract/toolforgeapi.Runner
// already named four of these operations the way the forge's users say them),
// so the module answers in those names and the daemon spells none of them.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the forge's lifecycle any more;
//   - the daemon still holds the store's FILE and the module's FACE.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"testing"
)

// forgeForwardersInTheDaemon are the daemon's spellings of the forge's
// operations.
var forgeForwardersInTheDaemon = []string{
	"DraftScriptTool",
	"UpdateScriptTool",
	"TestScriptTool",
	"PromoteScriptTool",
	"RequestToolPromotion",
	"QuarantineScriptTool",
	"RemoveScriptTool",
}

// TestTheDaemonDoesNotSpellOutTheForgeLifecycle pins the move: the forge's
// lifecycle belongs to the extensions module, and the daemon keeps the
// store's file and the module's face.
func TestTheDaemonDoesNotSpellOutTheForgeLifecycle(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range forgeForwardersInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the forge's lifecycle is the extensions module's, and one operation has one name", name)
		}
	}
}

// TestTheDaemonKeepsTheForgeFileAndTheFace pins what stayed: the forge's
// FILE is the host's business (W3.45 — the host opens the catalogue's file),
// and the FACE is the module's runner.
func TestTheDaemonKeepsTheForgeFileAndTheFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"ToolForge", "ForgeRunner"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the forge's file and the module's face", name)
		}
	}
}
