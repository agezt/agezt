// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime script-tool forge: the READ over the forge's port +
//             the forwarders to the extensions module's forge service.
//
// The lifecycle is the EXTENSIONS MODULE's (W3.72): the module keeps the
// forge's operations — draft, edit, test, promote, ask the operator,
// quarantine, remove — and the bridge that folds the promoted scripts into a
// run's tool map, and the host holds the module's face
// (extapi.ForgeService) and forwards to it.
//
// The host's public names did not move — DraftScriptTool and its six
// siblings are still the daemon's verbs, and every caller outside the
// runtime is untouched. What moved is WHERE the answer comes from: the
// daemon used to hold the store AND spell out the lifecycle over it, which
// left the module keeping the CATALOGUE and the daemon keeping the
// OPERATIONS.
//
// The READ stays on this side of the line on purpose: it is the nil-safe
// view over the CONTRACT's port (W3.45) — the daemon holds the port, and a
// host with no forge answers "no tools" instead of calling into nothing.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// ErrNoForge is answered when the daemon handed the kernel no script-tool
// forge (W3.45). The forge's FILE is the daemon's business, so a kernel
// without one is a daemon with no script tools — and the writes must say so
// rather than call into nothing.
//
// The sentinel is the CONTRACT's row (toolforgeapi.ErrNoStore) since W3.72:
// the module raises it when the daemon handed over no forge store, and the
// host raises it when the module was never wired — and a caller compares with
// errors.Is, so both sides must raise the SAME error.
var ErrNoForge = toolforgeapi.ErrNoStore

// ToolForge returns the durable script-tool store (M794) — the contract's
// port (W3.45), which the host opened and injected.
//
// Nil when the host wired no forge: every operation below then reports that
// the forge is unavailable rather than calling into nothing.
func (k *Kernel) ToolForge() toolforgeapi.HostStore { return k.toolForge }

// SetForge injects the forge service the daemon drives.
//
// The forge is the EXTENSIONS MODULE's service: the module keeps the
// lifecycle, the daemon keeps the names. The composition root builds it over
// the store the host opened and hands it over here.
func (k *Kernel) SetForge(s extapi.ForgeService) {
	k.configMu.Lock()
	k.scriptForge = s
	k.configMu.Unlock()
}

// forgeService returns the injected forge service, or nil.
func (k *Kernel) forgeService() extapi.ForgeService {
	k.configMu.Lock()
	defer k.configMu.Unlock()
	return k.scriptForge
}

// forgeOrRefusal returns the forge service the daemon drives, or the
// refusal.
//
// The refusal is the contract's row: a daemon with no forge answers no
// forge, whether the store was never opened or the module was never wired.
func (k *Kernel) forgeOrRefusal() (extapi.ForgeService, error) {
	svc := k.forgeService()
	if svc == nil {
		return nil, toolforgeapi.ErrNoStore
	}
	return svc, nil
}

// DraftScriptTool validates and persists a new DRAFT script tool, journaling
// scripttool.created so the tool's birth is auditable.
func (k *Kernel) DraftScriptTool(corr string, st toolforgeapi.ScriptTool) (toolforgeapi.ScriptTool, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	return svc.Draft(corr, st)
}

// UpdateScriptTool edits a script tool's mutable fields via mutate,
// journaling scripttool.updated. The store demotes the tool to draft and
// clears its test record when the code/language changed — the payload says
// so, so `agt why` can explain why a live tool went dark. Returns false +
// nil error for an unknown ref (standing pattern).
func (k *Kernel) UpdateScriptTool(corr, ref string, mutate func(*toolforgeapi.ScriptTool)) (toolforgeapi.ScriptTool, bool, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, false, err
	}
	return svc.Update(corr, ref, mutate)
}

// TestScriptTool runs a script tool's CURRENT code once in the code-exec
// sandbox with input as the call payload (surfaced to the script as
// ./stdin.txt), records the verdict on the tool (Promote requires a pass),
// and journals scripttool.tested. The sandbox output comes back verbatim so
// the author can iterate.
func (k *Kernel) TestScriptTool(ctx context.Context, corr, ref, input string) (toolforgeapi.ScriptTool, string, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, "", err
	}
	return svc.Test(ctx, corr, ref, input)
}

// PromoteScriptTool moves a tested draft/quarantined tool to ACTIVE — from
// the next run on, every agent is offered it as forge_<name>. Journals
// scripttool.promoted.
func (k *Kernel) PromoteScriptTool(corr, ref string) (toolforgeapi.ScriptTool, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	return svc.Promote(corr, ref)
}

// RequestToolPromotion (M813): the agent ASKS for its tool to go live — the
// promotion queue. The request blocks on the HITL approval registry (it
// shows up in `agt approvals` and the console's Approvals view); a grant
// promotes through the exact path the operator CLI uses, a deny/timeout
// comes back as the decision, not an error. The "only tested code goes
// live" invariant is checked up front so an untested draft never even
// reaches the operator's queue.
func (k *Kernel) RequestToolPromotion(ctx context.Context, corr, ref string) (toolforgeapi.ScriptTool, approvalapi.Decision, string, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, "", "", err
	}
	return svc.RequestPromotion(ctx, corr, ref)
}

// QuarantineScriptTool pulls an active tool from production — the kill
// switch. Journals scripttool.quarantined with the operator's reason.
func (k *Kernel) QuarantineScriptTool(corr, ref, reason string) (toolforgeapi.ScriptTool, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return toolforgeapi.ScriptTool{}, err
	}
	return svc.Quarantine(corr, ref, reason)
}

// RemoveScriptTool deletes a script tool, journaling scripttool.removed when
// it existed. Returns whether it existed.
func (k *Kernel) RemoveScriptTool(corr, ref string) (bool, error) {
	svc, err := k.forgeOrRefusal()
	if err != nil {
		return false, err
	}
	return svc.Remove(corr, ref)
}

// mergeScriptTools returns the run's tool map extended with the forge's
// ACTIVE scripts as callable forge_<name> tools.
//
// The rule is the module's (W3.72): a registered tool wins a name collision,
// and the common no-scripts path stays allocation-free — the input map comes
// back untouched.
func (k *Kernel) mergeScriptTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	svc := k.forgeService()
	if svc == nil {
		return tools
	}
	return svc.MergeTools(tools)
}
