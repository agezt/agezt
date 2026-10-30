// SPDX-License-Identifier: MIT

package toolforge

// Provenance: the SCRIPT-TOOL BRIDGE (W3.72): MergeTools + the forged tool
//             adapter + the callable-name rule. Moved out of kernel/runtime
//             with the lifecycle — the bridge answers for the forge's ACTIVE
//             rows, so it belongs with them.
//
// The rule of the bridge is the same wherever it lives: a registered tool
// always wins a name collision, and a promoted script is offered under the
// forge_ prefix, which also lets the Edict toolmap route every forged call
// to the `code.exec` capability.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
	"github.com/agezt/agezt/kernel/contract/toolregapi"
)

// MergeTools returns the run's tool map extended with the forge's ACTIVE
// scripts as callable forge_<name> tools. Registered tools always win a name
// collision (the forge_ prefix makes one effectively impossible, but a script
// must never shadow a real tool). Returns the input map untouched when there
// is nothing to offer, so the common no-scripts path stays allocation-free.
func (s *Service) MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	if s.store == nil || s.runner == nil {
		return tools
	}
	active := s.store.Active()
	if len(active) == 0 {
		return tools
	}
	out := make(map[string]toolapi.Tool, len(tools)+len(active))
	for name, t := range tools {
		out[name] = t
	}
	for _, st := range active {
		name := ForgedToolName(st.Name)
		if _, exists := out[name]; exists {
			continue
		}
		out[name] = forgedTool{st: st, runner: s.runner}
	}
	return out
}

// ForgedToolName is the callable name of a promoted script: the prefix both
// namespaces scripts away from built-in tools and lets the Edict toolmap
// route every forged call to the `code.exec` capability.
func ForgedToolName(name string) string { return "forge_" + name }

// defaultForgedSchema is offered when the author supplied no input schema:
// a permissive object — the whole call input reaches the script as JSON.
const defaultForgedSchema = `{
  "type": "object",
  "additionalProperties": true
}`

// forgedTool adapts one ACTIVE script-tool record to toolapi.Tool: the call's
// raw JSON input rides into the sandbox (the script reads it from
// ./stdin.txt) and combined stdout+stderr comes back as the result.
type forgedTool struct {
	st     toolforgeapi.ScriptTool
	runner toolregapi.ScriptRunner
}

func (t forgedTool) Definition() toolapi.ToolDef {
	schema := strings.TrimSpace(t.st.InputSchema)
	if schema == "" {
		schema = defaultForgedSchema
	}
	desc := strings.TrimSpace(t.st.Description) +
		" (Forged script tool: a vetted " + t.st.Language +
		" script run in the sandbox; this call's JSON input is available to it as ./stdin.txt.)"
	return toolapi.ToolDef{
		Name:        ForgedToolName(t.st.Name),
		Description: desc,
		InputSchema: json.RawMessage(schema),
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectCompensable,
			PredictedEffects: []string{
				"Execute a promoted agent-authored script inside the configured sandbox.",
			},
			AffectedResources: []string{"code-exec sandbox", "script tool " + t.st.Name},
			RollbackNotes:     "Sandbox scratch output is disposable; any persistent or external effect depends on the sandbox profile and must be compensated manually.",
			Confidence:        0.6,
		},
	}
}

func (t forgedTool) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	in := string(raw)
	if strings.TrimSpace(in) == "" {
		in = "{}"
	}
	out, isErr, err := t.runner.RunScript(ctx, t.st.Language, t.st.Code, in)
	if err != nil {
		return toolapi.Result{Output: ForgedToolName(t.st.Name) + ": " + err.Error(), IsError: true}, nil
	}
	return toolapi.Result{Output: out, IsError: isErr}, nil
}
