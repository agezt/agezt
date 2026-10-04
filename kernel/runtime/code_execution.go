// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/ulid"
)

// codeExecution adapts the already-wired sandbox runner, not the public
// code_exec tool's wider project/files/packages API. It is private to one call:
// neither the registry nor the runner's execution/network settings are changed.
type codeExecution struct {
	runner CodeExecutor
	ran    bool
}

type codeExecutionInput struct {
	Language string `json:"language"`
	Code     string `json:"code"`
	Input    string `json:"input"`
}

func (*codeExecution) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:        "code_exec",
		Capability:  toolapi.ToolCapability{Name: string(edict.CapCodeExec)},
		Description: "Run code through the configured sandbox runner.",
		InputSchema: json.RawMessage(`{"type":"object","required":["language","code","input"],"properties":{"language":{"type":"string"},"code":{"type":"string"},"input":{"type":"string"}}}`),
		Effect: toolapi.ToolEffect{
			Class:             toolapi.EffectIrreversible,
			PredictedEffects:  []string{"write and execute code in the sandbox workspace", "may consume compute and contact the network when enabled"},
			AffectedResources: []string{"configured code-exec sandbox"},
			RollbackNotes:     "Sandbox cleanup does not undo external effects of executed code.",
			Confidence:        0.55,
		},
	}
}

func (x *codeExecution) LookupTool(name string) (toolapi.Tool, bool) {
	if name != "code_exec" {
		return nil, false
	}
	return x, true
}

func (x *codeExecution) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	var in codeExecutionInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return toolapi.Result{}, err
	}
	x.ran = true
	out, isErr, err := x.runner.RunScript(ctx, in.Language, in.Code, in.Input)
	return toolapi.Result{Output: out, IsError: isErr}, err
}

// ran records entry into the executor, including errors/panics, and remains
// false when policy or mandatory pre-invocation audit refuses the call.
func (k *Kernel) runCode(ctx context.Context, corr, callPrefix string, runner CodeExecutor, language, code, input string) (res toolapi.Result, ran bool, err error) {
	x := &codeExecution{runner: runner}
	args, err := json.Marshal(codeExecutionInput{Language: language, Code: code, Input: input})
	if err != nil {
		return toolapi.Result{}, false, err
	}
	res, err = k.RunToolWithLookup(ctx, corr, callPrefix+"-"+ulid.New(), "code_exec", args, x)
	return res, x.ran, err
}
