// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime script-tool forge: the host's face for the forge.
//
// The forge's LIFECYCLE is the EXTENSIONS MODULE's (W3.72): the module keeps
// the operations — draft, edit, test, promote, ask the operator, quarantine,
// remove — and the bridge that folds the promoted scripts into a run's tool
// map.
//
// The daemon used to hold SEVEN forwarders over the module's own operations
// (DraftScriptTool, UpdateScriptTool, TestScriptTool, PromoteScriptTool,
// RequestToolPromotion, QuarantineScriptTool, RemoveScriptTool) — a second
// spelling of each, and a second copy of the whole forge surface for the
// application layer and the plugin to call through.
//
// The forge's vocabulary is the CONTRACT's: the contract's runner port
// (kernel/contract/toolforgeapi.Runner) already named four of these
// operations the way the forge's users say them, and the module answers in
// those names (W3.76t). The daemon spells none of them.
//
// What the daemon keeps is the HOST's own business:
//
//   - the forge's FILE (ToolForge): the store port the daemon opened
//     (W3.45), and the nil-safe read view over it;
//   - the module's FACE (ForgeRunner): the runner the application layer and
//     the plugin bind.
//
// The composition root hands the kernel the BUILDER (see
// runtime.Config.ForgeBuilder) and the kernel calls it while opening itself —
// so the daemon never needs a setter for it.

import (
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// ErrNoForge is the row a caller compares with when the script-tool forge
// is not there (W3.45).
//
// The sentinel is the CONTRACT's row (toolforgeapi.ErrNoStore): the module
// raises it when the daemon handed over no forge store, and the daemon's
// alias lets a caller ask the daemon's question in the daemon's own name —
// a caller compares with errors.Is, so both sides must raise the SAME error.
//
// The daemon no longer holds the forge's verbs (W3.76t), so it no longer
// raises this row itself: the refusal belongs to the module and to the
// application layer that asks it.
var ErrNoForge = toolforgeapi.ErrNoStore

// ToolForge returns the durable script-tool store (M794) — the contract's
// port (W3.45), which the host opened and injected.
//
// Nil when the host wired no forge: the reads then answer "no tools" instead
// of calling into nothing.
func (k *Kernel) ToolForge() toolforgeapi.HostStore { return k.toolForge }

// ForgeRunner returns the forge's runner — the EXTENSIONS MODULE's face.
//
// It is what the application layer binds (W3.76t): the tools application asks
// the forge, not the daemon. A daemon that was given no builder holds a nil
// face, and the application refuses rather than calling into nothing.
func (k *Kernel) ForgeRunner() extapi.ForgeRunner {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.scriptForge
}

// mergeScriptTools returns the run's tool map extended with the forge's
// ACTIVE scripts as callable forge_<name> tools.
//
// The rule is the module's (W3.72): a registered tool wins a name collision,
// and the common no-scripts path stays allocation-free — the input map comes
// back untouched.
func (k *Kernel) mergeScriptTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	svc := k.ForgeRunner()
	if svc == nil {
		return tools
	}
	return svc.MergeTools(tools)
}
