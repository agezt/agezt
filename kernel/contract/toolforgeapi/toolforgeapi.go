// SPDX-License-Identifier: MIT

// Package toolforgeapi is the contract layer's home for the script-tool rows:
// one named, durable script the agent can call as a tool, the lifecycle state
// it is in, the rule that decides whether it may be drafted, and the port
// through which the agent-facing tool reads the stored ones.
//
// The rows live here because the tool that publishes them is a PLUGIN, and a
// plugin may reach the contract and the platform layers and nothing else. The
// implementation that keeps them — the store, the runner that executes them
// in the sandbox — stays in the module
// (kernel/modules/extensions/toolforge); the module keeps the names its own
// callers use as aliases, so there is one row and one spelling of it.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package toolforgeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
)

// Status is a script tool's lifecycle state.
type Status string

const (
	// StatusDraft: authored but not live — never offered to runs.
	StatusDraft Status = "draft"
	// StatusActive: promoted — offered to every run as forge_<name>.
	StatusActive Status = "active"
	// StatusQuarantined: pulled from production (kill switch); re-promotable.
	StatusQuarantined Status = "quarantined"
)

// ScriptTool is one named, durable script the agent can call as a tool.
type ScriptTool struct {
	ID string `json:"id"`
	// Name is the tool's immutable handle; runs see it as `forge_<name>`.
	Name string `json:"name"`
	// Description is what the MODEL reads to decide when to call it.
	Description string `json:"description"`
	// Language is the sandbox runtime id ("python", "node", "deno").
	// Availability is judged at test/run time by the sandbox itself.
	Language string `json:"language"`
	// Code is the script body. Contract: the invoking call's JSON input is
	// written to ./stdin.txt in the work dir; stdout becomes the tool result.
	Code string `json:"code"`
	// InputSchema is an optional JSON-Schema object describing the call
	// input; empty means a permissive default schema.
	InputSchema string `json:"input_schema,omitempty"`

	Status Status `json:"status"`
	// TestedOK records whether the CURRENT code has a passing sandbox test;
	// cleared by any code/language edit. Promote requires it.
	TestedOK bool  `json:"tested_ok"`
	TestedMS int64 `json:"tested_ms,omitempty"`

	CreatedMS int64 `json:"created_ms"`
	UpdatedMS int64 `json:"updated_ms"`
}

// nameRe: a tool-name token — lowercase start, then letters/digits/underscore.
// `forge_` + 40 chars stays well inside provider tool-name limits.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// langRe: a sandbox runtime id.
var langRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

// The size caps the rule enforces.
const (
	MaxCodeBytes   = 128 * 1024 // a tool, not a codebase
	MaxDescBytes   = 2 * 1024
	MaxSchemaBytes = 16 * 1024
)

// Validate checks a script tool's user-supplied fields (identity/lifecycle
// fields are kernel-assigned and not judged here).
func Validate(st ScriptTool) error {
	if !nameRe.MatchString(st.Name) {
		return fmt.Errorf("toolforge: name must match %s", nameRe)
	}
	if strings.TrimSpace(st.Description) == "" {
		return errors.New("toolforge: description is required (the model reads it to decide when to call the tool)")
	}
	if len(st.Description) > MaxDescBytes {
		return fmt.Errorf("toolforge: description exceeds %d bytes", MaxDescBytes)
	}
	if !langRe.MatchString(st.Language) {
		return errors.New("toolforge: language is required (a sandbox runtime id, e.g. python/node/deno)")
	}
	if strings.TrimSpace(st.Code) == "" {
		return errors.New("toolforge: code is required")
	}
	if len(st.Code) > MaxCodeBytes {
		return fmt.Errorf("toolforge: code exceeds %d bytes", MaxCodeBytes)
	}
	if s := strings.TrimSpace(st.InputSchema); s != "" {
		if len(s) > MaxSchemaBytes {
			return fmt.Errorf("toolforge: input_schema exceeds %d bytes", MaxSchemaBytes)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(s), &obj); err != nil {
			return fmt.Errorf("toolforge: input_schema must be a JSON object: %w", err)
		}
	}
	return nil
}

// Store is the port through which the stored script tools are read.
//
// The implementation — the on-disk store in the extensions module — satisfies
// it; the agent-facing tool never names the implementation, because it is a
// plugin.
//
// It stays TWO verbs wide on purpose: a plugin holds every method of the port
// it is given, and a plugin that could Promote a tool would be able to put
// its own code in front of the operator's agent. The host's verbs live on
// HostStore.
type Store interface {
	// Get returns the tool at ref; the bool is false if absent.
	Get(ref string) (ScriptTool, bool)
	// List returns every stored tool.
	List() []ScriptTool
}

// HostStore is the HOST-facing port: the two read verbs a plugin holds, plus
// the lifecycle verbs the host drives — draft, edit, record a test, promote,
// quarantine, remove, and list what is active.
//
// The split exists because the two audiences are not the same. A plugin reads
// the catalogue. The host runs the lifecycle, and it is the host that asks
// the operator before a tool goes live.
//
// One store, two views of it: the daemon opens the file and hands the kernel
// this port; the tool registry receives the narrower Store.
type HostStore interface {
	Store
	// Add saves a new tool and returns it.
	Add(st ScriptTool) (ScriptTool, error)
	// Update applies mutate to the tool at ref.
	Update(ref string, mutate func(*ScriptTool)) (ScriptTool, error)
	// RecordTest writes the outcome of a sandbox run against the tool at ref.
	RecordTest(ref string, ok bool) (ScriptTool, error)
	// Promote makes the tool at ref live.
	Promote(ref string) (ScriptTool, error)
	// Quarantine takes the tool at ref out of circulation.
	Quarantine(ref string) (ScriptTool, error)
	// Remove deletes the tool at ref; the bool is false if it was already
	// gone.
	Remove(ref string) (ScriptTool, bool, error)
	// Active returns the tools that are live.
	Active() []ScriptTool
	// Count returns how many tools the store holds.
	Count() int
}

// ErrNotFound is returned when the store holds no tool at the given ref.
//
// It is the CONTRACT's row because the host answers with it: a caller that
// asks for a tool that is not there must be able to tell "no such tool" from
// "the store failed", and the host may not name the module's sentinel.
var ErrNotFound = errors.New("toolforge: script tool not found")

// ErrUntested is returned when a tool is asked to go live without a passing
// test behind it.
//
// It is the CONTRACT's row for the same reason as ErrNotFound: the promotion
// flow answers with it, and the flow lives in the host.
var ErrUntested = errors.New("toolforge: the current code has no passing test — run a test first")

// Runner is the port through which a script tool's LIFECYCLE is driven:
// draft it, edit it, test it in the sandbox, and ask the operator to promote
// it.
//
// It is the contract's port, and the tool that drives it is a plugin — so the
// row the registry carries cannot be the live kernel. The implementation —
// the kernel's own forge methods — satisfies it without an adapter: its
// signatures already speak the contract's rows (`toolforge.ScriptTool` is an
// alias of `toolforgeapi.ScriptTool`, and `approval.Decision` is an alias of
// `approvalapi.Decision`).
type Runner interface {
	// DraftScriptTool saves a new draft and returns it.
	DraftScriptTool(corr string, st ScriptTool) (ScriptTool, error)
	// UpdateScriptTool applies mutate to the tool at ref; the bool is
	// false when no such tool exists.
	UpdateScriptTool(corr, ref string, mutate func(*ScriptTool)) (ScriptTool, bool, error)
	// TestScriptTool runs the tool at ref once in the sandbox with the
	// given input and returns the tool and the run's output.
	TestScriptTool(ctx context.Context, corr, ref, input string) (ScriptTool, string, error)
	// RequestToolPromotion asks the operator to take the tool at ref live.
	// It answers the operator's verdict, a human reason and an error.
	RequestToolPromotion(ctx context.Context, corr, ref string) (ScriptTool, approvalapi.Decision, string, error)
}
