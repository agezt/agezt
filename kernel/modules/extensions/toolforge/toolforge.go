// SPDX-License-Identifier: MIT

package toolforge

// Provenance: Package toolforge is the script-tool forge (M794): agent-authored code
//             promoted into durable, callable tools — the close of the
//             write→use→improve cycle. An agent (or operator) DRAFTS a named script,
//             TESTS it in the code-exec sandbox, and once a test of the current code
//             has passed the OPERATOR promotes it; from then on every run is offered
//             the script as a real tool named `forge_<name>`, executed through the
//             same warden-isolated, secret-scrubbed sandbox as `code_exec` and gated
//             by the same `code.exec` Edict capability. Quarantine is the instant
//             kill switch, and ANY edit to the code demotes the tool back to draft
//             with its test record cleared — only tested code is ever live. Storage
//             mirrors kernel/roster: a single JSON file rewritten atomically on
//             change, safe for concurrent use; every lifecycle mutation is journaled
//             by the kernel (scripttool.*) so `agt why` can explain how a tool came
//             to be. The Store API (Open + Add + Update + RecordTest + Promote +
//             Quarantine + Remove + Get + List + Active + Count + save) lives in
//             toolforge_store.go. Extracted from toolforge.go during the Day-203
//             god-file split. Public API unchanged.

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
)

// safeCall runs fn under a panic-recovery defer. If fn panics, the panic is caught
// and returned as an error so callers never receive a raw panic.
func safeCall(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return fn()
}

// ErrNotFound is returned for an unknown script-tool id/name.
var ErrNotFound = errors.New("toolforge: script tool not found")

// ErrUntested is returned by Promote when the CURRENT code has no passing
// test on record — the forge's core invariant: only tested code goes live.
var ErrUntested = errors.New("toolforge: the current code has no passing test — run a test first")

// Status is a script tool's lifecycle state.
//
// The row itself lives in the contract layer (kernel/contract/toolforgeapi);
// the name stays here as an alias, so every existing caller is unchanged.
type Status = toolforgeapi.Status

const (
	// StatusDraft: authored but not live — never offered to runs.
	StatusDraft = toolforgeapi.StatusDraft
	// StatusActive: promoted — offered to every run as forge_<name>.
	StatusActive = toolforgeapi.StatusActive
	// StatusQuarantined: pulled from production (kill switch); re-promotable.
	StatusQuarantined = toolforgeapi.StatusQuarantined
)

// ScriptTool is one named, durable script the agent can call as a tool.
//
// The row itself lives in the contract layer (kernel/contract/toolforgeapi);
// the name stays here as an alias, so every existing caller is unchanged.
type ScriptTool = toolforgeapi.ScriptTool

// Runner executes a script in the code-exec sandbox: the call's raw JSON
// input rides in as inputJSON (surfaced to the script as ./stdin.txt) and
// combined stdout+stderr comes back. isError mirrors the sandbox's own
// verdict (non-zero exit, timeout, unavailable language...). Implemented by
// the code_exec tool; the daemon is the single wiring point.
type Runner interface {
	RunScript(ctx context.Context, language, code, inputJSON string) (output string, isError bool, err error)
}

// The caps the rule enforces: the name pattern, the language pattern and the
// size limits live in the contract layer, with the rule. The module keeps the
// size caps as aliases, because its own tests measure against them.
const (
	maxCodeBytes   = toolforgeapi.MaxCodeBytes // a tool, not a codebase
	maxDescBytes   = toolforgeapi.MaxDescBytes
	maxSchemaBytes = toolforgeapi.MaxSchemaBytes
)

// Validate checks a script tool's user-supplied fields (identity/lifecycle
// fields are kernel-assigned and not judged here).
//
// The rule lives in the contract layer (kernel/contract/toolforgeapi) — it is
// pure, and the agent-facing tool (a plugin) drafts through it. This is a
// delegation, so there is one rule.
func Validate(st ScriptTool) error { return toolforgeapi.Validate(st) }
