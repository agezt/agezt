// SPDX-License-Identifier: MIT

// Package toolforge is the script-tool forge (M794): agent-authored code
// promoted into durable, callable tools — the close of the write→use→improve
// cycle. An agent (or operator) DRAFTS a named script, TESTS it in the
// code-exec sandbox, and once a test of the current code has passed the
// OPERATOR promotes it; from then on every run is offered the script as a
// real tool named `forge_<name>`, executed through the same warden-isolated,
// secret-scrubbed sandbox as `code_exec` and gated by the same `code.exec`
// Edict capability. Quarantine is the instant kill switch, and ANY edit to
// the code demotes the tool back to draft with its test record cleared —
// only tested code is ever live.
//
// Storage mirrors kernel/roster: a single JSON file rewritten atomically on
// change, safe for concurrent use; every lifecycle mutation is journaled by
// the kernel (scripttool.*) so `agt why` can explain how a tool came to be.
//
// The Store API (Open + Add + Update + RecordTest + Promote + Quarantine +
// Remove + Get + List + Active + Count + save) lives in toolforge_store.go.
// Extracted from toolforge.go during the Day-203 god-file split.
// Public API unchanged.
package toolforge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

// Runner executes a script in the code-exec sandbox: the call's raw JSON
// input rides in as inputJSON (surfaced to the script as ./stdin.txt) and
// combined stdout+stderr comes back. isError mirrors the sandbox's own
// verdict (non-zero exit, timeout, unavailable language...). Implemented by
// the code_exec tool; the daemon is the single wiring point.
type Runner interface {
	RunScript(ctx context.Context, language, code, inputJSON string) (output string, isError bool, err error)
}

// nameRe: a tool-name token — lowercase start, then letters/digits/underscore.
// `forge_` + 40 chars stays well inside provider tool-name limits.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

var langRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)

const (
	maxCodeBytes   = 128 * 1024 // a tool, not a codebase
	maxDescBytes   = 2 * 1024
	maxSchemaBytes = 16 * 1024
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
	if len(st.Description) > maxDescBytes {
		return fmt.Errorf("toolforge: description exceeds %d bytes", maxDescBytes)
	}
	if !langRe.MatchString(st.Language) {
		return errors.New("toolforge: language is required (a sandbox runtime id, e.g. python/node/deno)")
	}
	if strings.TrimSpace(st.Code) == "" {
		return errors.New("toolforge: code is required")
	}
	if len(st.Code) > maxCodeBytes {
		return fmt.Errorf("toolforge: code exceeds %d bytes", maxCodeBytes)
	}
	if s := strings.TrimSpace(st.InputSchema); s != "" {
		if len(s) > maxSchemaBytes {
			return fmt.Errorf("toolforge: input_schema exceeds %d bytes", maxSchemaBytes)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(s), &obj); err != nil {
			return fmt.Errorf("toolforge: input_schema must be a JSON object: %w", err)
		}
	}
	return nil
}

