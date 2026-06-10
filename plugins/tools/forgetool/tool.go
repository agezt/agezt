// SPDX-License-Identifier: MIT

// Package forgetool is the in-process `tool_forge` tool (M794): the agent
// builds its OWN tools. It drafts a named script (Python/Node/Deno), tests
// it in the code-exec sandbox, and iterates; once a test of the current code
// passes, the OPERATOR promotes it (`agt toolforge promote` / the console)
// and from then on every run can call it as a real `forge_<name>` tool.
// Authoring is gated by the `tool.forge` Edict capability; op=test executes
// code and is gated by `code.exec`. Every transition is journaled
// (scripttool.*) and quarantine is the operator's instant kill switch.
package forgetool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/toolforge"
)

// Kernel is the slice of the runtime kernel this tool drives — satisfied by
// *runtime.Kernel and easy to fake in tests.
type Kernel interface {
	DraftScriptTool(corr string, st toolforge.ScriptTool) (toolforge.ScriptTool, error)
	UpdateScriptTool(corr, ref string, mutate func(*toolforge.ScriptTool)) (toolforge.ScriptTool, bool, error)
	TestScriptTool(ctx context.Context, corr, ref, input string) (toolforge.ScriptTool, string, error)
	RequestToolPromotion(ctx context.Context, corr, ref string) (toolforge.ScriptTool, approval.Decision, string, error)
	ToolForge() *toolforge.Store
}

// Tool implements agent.Tool. Construct with New, then Bind the live kernel
// once it opens (the daemon is the single wiring point).
type Tool struct {
	mu sync.RWMutex
	k  Kernel
}

// New returns an unbound Tool — Invoke reports the forge unavailable until
// Bind is called.
func New() *Tool { return &Tool{} }

// Bind wires the live kernel. Called once after the kernel opens.
func (t *Tool) Bind(k Kernel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.k = k
}

func (t *Tool) current() Kernel {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.k
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "tool_forge",
		Description: "Build your own durable tools out of code. " +
			"op=draft saves a named script (python/node/deno) as a DRAFT tool; " +
			"op=test runs the draft once in the sandbox with a sample JSON input (your script reads it from ./stdin.txt and must print its result to stdout); " +
			"op=update edits a draft (changing the code clears its test record and demotes an active tool back to draft); " +
			"op=request_promotion asks the human operator to take a TESTED draft live — the call waits for their decision; " +
			"op=list and op=show inspect your tools. " +
			"A draft only goes LIVE when the operator approves — after a passing test — and is then callable by every agent as forge_<name>. " +
			"Use this when you've written code worth keeping: turn it into a tool instead of rewriting it every run.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":           {"type":"string", "enum":["draft","update","test","request_promotion","list","show"]},
    "name":         {"type":"string", "description":"For op=draft: the tool's handle (lowercase letters/digits/underscore, e.g. \"fetch_weather\"); callable as forge_<name> once promoted."},
    "description":  {"type":"string", "description":"For op=draft/update: what the tool does and when to call it — the model-facing description."},
    "language":     {"type":"string", "description":"For op=draft/update: the sandbox runtime, e.g. \"python\", \"node\", or \"deno\"."},
    "code":         {"type":"string", "description":"For op=draft/update: the script. Contract: the call's JSON input is in ./stdin.txt; print the result to stdout; exit non-zero on failure."},
    "input_schema": {"type":"string", "description":"For op=draft/update (optional): a JSON-Schema object (as a string) describing the tool's input."},
    "ref":          {"type":"string", "description":"For op=update/test/show: the tool's id or name."},
    "input":        {"type":"string", "description":"For op=test (optional): a sample JSON input for the run (default {})."}
  }
}`),
	}
}

type input struct {
	Op          string `json:"op"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"`
	Code        string `json:"code"`
	InputSchema string `json:"input_schema"`
	Ref         string `json:"ref"`
	Input       string `json:"input"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("tool_forge: parse input: %w", err)
	}
	k := t.current()
	if k == nil {
		return errResult("the tool forge is not available on this daemon"), nil
	}
	corr := agent.CorrelationFromContext(ctx)

	switch in.Op {
	case "draft":
		st, err := k.DraftScriptTool(corr, toolforge.ScriptTool{
			Name:        in.Name,
			Description: in.Description,
			Language:    in.Language,
			Code:        in.Code,
			InputSchema: in.InputSchema,
		})
		if err != nil {
			return errResult(err.Error()), nil
		}
		v := view(st)
		v["message"] = "drafted — test it (op=test), then ask the operator to promote it"
		return okJSON(v), nil

	case "update":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=update needs a "ref" (the tool's id or name)`), nil
		}
		st, found, err := k.UpdateScriptTool(corr, in.Ref, func(dst *toolforge.ScriptTool) {
			if strings.TrimSpace(in.Description) != "" {
				dst.Description = in.Description
			}
			if strings.TrimSpace(in.Language) != "" {
				dst.Language = in.Language
			}
			if in.Code != "" {
				dst.Code = in.Code
			}
			if strings.TrimSpace(in.InputSchema) != "" {
				dst.InputSchema = in.InputSchema
			}
		})
		if err != nil {
			return errResult(err.Error()), nil
		}
		if !found {
			return errResult("no script tool " + in.Ref), nil
		}
		v := view(st)
		msg := "updated"
		if !st.TestedOK {
			msg = "updated — the code changed, so it's a draft again; re-test before asking for promotion"
		}
		v["message"] = msg
		return okJSON(v), nil

	case "test":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=test needs a "ref" (the tool's id or name)`), nil
		}
		st, out, err := k.TestScriptTool(ctx, corr, in.Ref, in.Input)
		if err != nil {
			return errResult(err.Error()), nil
		}
		verdict := "PASSED — request promotion (op=request_promotion) or ask the operator (agt toolforge promote " + st.Name + ")"
		if !st.TestedOK {
			verdict = "FAILED — fix the code (op=update) and test again"
		}
		return agent.Result{Output: "test " + verdict + "\n\n" + out, IsError: !st.TestedOK}, nil

	case "request_promotion":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=request_promotion needs a "ref" (the tool's id or name)`), nil
		}
		st, decision, reason, err := k.RequestToolPromotion(ctx, corr, in.Ref)
		if err != nil {
			return errResult(err.Error()), nil
		}
		switch decision {
		case approval.DecisionGrant:
			v := view(st)
			v["message"] = "promoted by the operator — callable as forge_" + st.Name + " from the next run"
			return okJSON(v), nil
		case approval.DecisionDeny:
			msg := "promotion denied by the operator"
			if reason != "" {
				msg += ": " + reason
			}
			return errResult(msg + " — improve the tool or move on"), nil
		default:
			return errResult("promotion request " + string(decision) + " — the operator did not decide in time"), nil
		}

	case "list":
		all := k.ToolForge().List()
		views := make([]map[string]any, 0, len(all))
		for _, st := range all {
			views = append(views, view(st))
		}
		return okJSON(map[string]any{"count": len(views), "tools": views}), nil

	case "show":
		if strings.TrimSpace(in.Ref) == "" {
			return errResult(`op=show needs a "ref" (the tool's id or name)`), nil
		}
		st, found := k.ToolForge().Get(strings.TrimSpace(in.Ref))
		if !found {
			return errResult("no script tool " + in.Ref), nil
		}
		v := view(st)
		v["code"] = st.Code
		if st.InputSchema != "" {
			v["input_schema"] = st.InputSchema
		}
		return okJSON(v), nil

	case "":
		return errResult("op required (draft|update|test|request_promotion|list|show)"), nil
	default:
		return errResult("unknown op " + in.Op + " (draft|update|test|request_promotion|list|show)"), nil
	}
}

func view(st toolforge.ScriptTool) map[string]any {
	v := map[string]any{
		"id":        st.ID,
		"name":      st.Name,
		"language":  st.Language,
		"status":    string(st.Status),
		"tested_ok": st.TestedOK,
	}
	if st.Description != "" {
		v["description"] = st.Description
	}
	if st.Status == toolforge.StatusActive {
		v["callable_as"] = "forge_" + st.Name
	}
	return v
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "tool_forge: " + msg, IsError: true}
}
