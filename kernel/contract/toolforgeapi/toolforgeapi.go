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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
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
type Store interface {
	// Get returns the tool at ref; the bool is false if absent.
	Get(ref string) (ScriptTool, bool)
	// List returns every stored tool.
	List() []ScriptTool
}
