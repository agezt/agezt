// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Resolution retains the implementation and its trusted metadata, including on
// schema rejection. Callers keep their own unavailable/schema error formatting.
type Resolution struct {
	Tool       toolapi.Tool
	Definition toolapi.ToolDef
	Found      bool
	InputError error
}

// Resolve looks up a call once and validates input before guard/policy/memo.
// The lookup belongs to the caller (a loop's enabled set or the host registry).
// It performs no policy, audit or tool effects.
func Resolve(call llm.ToolCall, lookup func(string) (toolapi.Tool, bool)) Resolution {
	tool, found := lookup(call.Name)
	r := Resolution{Tool: tool, Found: found}
	if !found {
		return r
	}
	r.Definition = tool.Definition()
	r.InputError = schema.ValidateToolInput(r.Definition, call.Input)
	return r
}
