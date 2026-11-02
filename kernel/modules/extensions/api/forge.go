// SPDX-License-Identifier: MIT

package api

// The extensions module's public vocabulary for the SCRIPT-TOOL FORGE: the
// lifecycle the host drives over the agent-authored scripts and the bridge
// that hands the promoted ones to a run.
//
// The ROWS live in the contract layer (kernel/contract/toolforgeapi): the
// tool that drives the lifecycle is a PLUGIN, and a plugin may reach the
// contract and the platform layers only. The implementation — the store, the
// bridge — stays in the module (kernel/modules/extensions/toolforge).
//
// Rules of the road:
//
//   - Types, interfaces and ports only. No logic that needs a store, a bus or
//     a clock — those are the implementation package's business.
//   - One row, one home. A row both the host and the module name is declared
//     exactly once — in the contract package — and every other side aliases
//     it.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
	"github.com/agezt/agezt/kernel/event"
)

// ForgeRunner is the script-tool forge's public face: the lifecycle the host
// drives over the agent-authored scripts, and the bridge that folds the
// PROMOTED ones into a run's tool map.
//
// The lifecycle and the bridge are ONE face because they answer ONE question —
// which tools may this run call — and they read the same ACTIVE rows. A second
// face would be a second owner of the same catalogue.
//
// The verbs are the CONTRACT's names (W3.76t): the forge's own vocabulary
// already IS the contract's runner port ("draft a script tool", "test a script
// tool"), and the four the contract names are spelled the contract's way. The
// daemon used to hold seven forwarders over these very operations — a second
// spelling of each — and the application layer called the daemon for them.
// The module answers in the contract's names now, so the daemon spells none of
// them.
//
// The contract's port is EMBEDDED, not restated: the four operations the
// contract names (draft, edit, test, ask the operator) are the contract's
// row, and the module's service answers them directly. A second spelling
// here would be a second place to change a name.
type ForgeRunner interface {
	// Runner is the CONTRACT's port: draft a tool, edit it, test it in the
	// sandbox and ask the operator to promote it.
	toolforgeapi.Runner

	// PromoteScriptTool moves a tested draft/quarantined tool to ACTIVE — from the
	// next run on, every agent is offered it as forge_<name>. Journals
	// scripttool.promoted.
	PromoteScriptTool(corr, ref string) (toolforgeapi.ScriptTool, error)
	// QuarantineScriptTool pulls an active tool from production — the kill switch.
	// Journals scripttool.quarantined with the operator's reason.
	QuarantineScriptTool(corr, ref, reason string) (toolforgeapi.ScriptTool, error)
	// RemoveScriptTool deletes a script tool, journaling scripttool.removed when it
	// existed. Returns whether it existed.
	RemoveScriptTool(corr, ref string) (bool, error)
	// MergeTools returns the run's tool map extended with the forge's ACTIVE
	// scripts as forge_<name> tools. Registered tools win a name collision;
	// the input map comes back untouched when there is nothing to offer.
	MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool
}

// ForgeDependencies are the host's ports the forge needs to answer a verb.
//
// Every port is a way to ASK, never a way to reach: the forge publishes on
// the host's bus, runs scripts through the host's sandbox and asks the
// operator through the host's approval registry.
type ForgeDependencies struct {
	// Store is the durable script-tool catalogue — the contract's store
	// port. The catalogue's FILE is the daemon's business (W3.45): the
	// composition root opens it and hands the port over.
	//
	// A nil port means the daemon has no forge, and the writes refuse
	// rather than call into nothing.
	Store toolforgeapi.HostStore

	// ScriptRunner executes a script in the code-exec sandbox.
	//
	// A nil runner means no script tool can be tested or called, and the
	// verbs that need it refuse with a readable error rather than calling
	// into nothing.
	ScriptRunner toolregapi.ScriptRunner

	// AutoPromote lets a tested draft go live immediately, without the
	// operator's queue. It is the host's switch (the daemon's config), and
	// the forge only obeys it.
	AutoPromote bool

	// Approve asks the OPERATOR for a verdict on a promotion request.
	//
	// A nil port means the host has no approval registry, and the promotion
	// request refuses with a readable error rather than calling into
	// nothing.
	Approve func(ctx context.Context, spec approvalapi.SubmitSpec) approvalapi.Outcome

	// Publish emits one event on the host's bus.
	//
	// The host owns the bus; the forge only says what happened to a script
	// tool, and the subject and the kind are the forge's own. A nil port
	// means the host has no bus, and the forge stays quiet rather than
	// publishing into nothing.
	Publish func(spec event.Spec)
}

// ForgeBuilder builds the forge's runner over the dependencies the host
// collected.
//
// The daemon hands over the BUILDER (it names the module); the host calls it
// with the store, the sandbox runner, the operator's approval registry and
// its own publisher — so every lifecycle transition journals through the
// host's bus without either side naming the other's implementation.
type ForgeBuilder func(ForgeDependencies) ForgeRunner
