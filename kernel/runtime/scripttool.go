// SPDX-License-Identifier: MIT

package runtime

// Script-tool forge integration (M794): the kernel-side lifecycle of
// agent-authored scripts becoming callable tools. The toolforge store holds
// the records; this file journals every transition (scripttool.*), runs
// sandbox tests through cfg.ScriptRunner, and merges the ACTIVE scripts into
// each run's tool map as `forge_<name>` tools. Execution rides the same
// code-exec sandbox (warden isolation, scrubbed env) and the same `code.exec`
// Edict capability as direct code execution — promotion changes WHO can be
// called, never what the sandbox allows.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolforge"
)

// ToolForge returns the durable script-tool store (M794). Always non-nil
// after Open.
func (k *Kernel) ToolForge() *toolforge.Store { return k.toolForge }

// DraftScriptTool validates and persists a new DRAFT script tool, journaling
// scripttool.created so the tool's birth is auditable.
func (k *Kernel) DraftScriptTool(corr string, st toolforge.ScriptTool) (toolforge.ScriptTool, error) {
	saved, err := k.toolForge.Add(st)
	if err != nil {
		return toolforge.ScriptTool{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "toolforge." + saved.Name, Kind: event.KindScriptToolCreated, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": saved.ID, "name": saved.Name, "language": saved.Language, "code_bytes": len(saved.Code)},
	})
	return saved, nil
}

// UpdateScriptTool edits a script tool's mutable fields via mutate,
// journaling scripttool.updated. The store demotes the tool to draft and
// clears its test record when the code/language changed — the payload says
// so, so `agt why` can explain why a live tool went dark. Returns false +
// nil error for an unknown ref (standing pattern).
func (k *Kernel) UpdateScriptTool(corr, ref string, mutate func(*toolforge.ScriptTool)) (toolforge.ScriptTool, bool, error) {
	st, err := k.toolForge.Update(ref, mutate)
	if errors.Is(err, toolforge.ErrNotFound) {
		return toolforge.ScriptTool{}, false, nil
	}
	if err != nil {
		return toolforge.ScriptTool{}, false, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolUpdated, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": st.ID, "name": st.Name, "status": string(st.Status), "tested_ok": st.TestedOK},
	})
	return st, true, nil
}

// TestScriptTool runs a script tool's CURRENT code once in the code-exec
// sandbox with input as the call payload (surfaced to the script as
// ./stdin.txt), records the verdict on the tool (Promote requires a pass),
// and journals scripttool.tested. The sandbox output comes back verbatim so
// the author can iterate.
func (k *Kernel) TestScriptTool(ctx context.Context, corr, ref, input string) (toolforge.ScriptTool, string, error) {
	if k.cfg.ScriptRunner == nil {
		return toolforge.ScriptTool{}, "", errors.New("runtime: script tools are not available on this daemon (no sandbox runner)")
	}
	st, found := k.toolForge.Get(ref)
	if !found {
		return toolforge.ScriptTool{}, "", toolforge.ErrNotFound
	}
	if strings.TrimSpace(input) == "" {
		input = "{}"
	}
	out, isErr, err := k.cfg.ScriptRunner.RunScript(ctx, st.Language, st.Code, input)
	if err != nil {
		return toolforge.ScriptTool{}, "", err
	}
	ok := !isErr
	rec, rerr := k.toolForge.RecordTest(st.ID, ok)
	if rerr != nil {
		return toolforge.ScriptTool{}, "", rerr
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolTested, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": st.ID, "name": st.Name, "ok": ok, "output_bytes": len(out)},
	})
	return rec, out, nil
}

// PromoteScriptTool moves a tested draft/quarantined tool to ACTIVE — from
// the next run on, every agent is offered it as forge_<name>. Journals
// scripttool.promoted.
func (k *Kernel) PromoteScriptTool(corr, ref string) (toolforge.ScriptTool, error) {
	st, err := k.toolForge.Promote(ref)
	if err != nil {
		return toolforge.ScriptTool{}, err
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolPromoted, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       map[string]any{"id": st.ID, "name": st.Name, "language": st.Language},
	})
	return st, nil
}

// RequestToolPromotion (M813): the agent ASKS for its tool to go live — the
// promotion queue. The request blocks on the HITL approval registry (it
// shows up in `agt approvals` and the console's Approvals view); a grant
// promotes through the exact path the operator CLI uses, a deny/timeout
// comes back as the decision, not an error. The "only tested code goes
// live" invariant is checked up front so an untested draft never even
// reaches the operator's queue.
func (k *Kernel) RequestToolPromotion(ctx context.Context, corr, ref string) (toolforge.ScriptTool, approval.Decision, string, error) {
	st, found := k.toolForge.Get(strings.TrimSpace(ref))
	if !found {
		return toolforge.ScriptTool{}, "", "", fmt.Errorf("toolforge: no script tool %q", ref)
	}
	if st.Status == toolforge.StatusActive {
		return st, "", "", fmt.Errorf("toolforge: %s is already active", st.Name)
	}
	if !st.TestedOK {
		return toolforge.ScriptTool{}, "", "", toolforge.ErrUntested
	}
	if k.cfg.AutoPromoteScriptTools {
		promoted, err := k.PromoteScriptTool(corr, st.Name)
		if err != nil {
			return st, approval.DecisionGrant, "auto-promote enabled", err
		}
		return promoted, approval.DecisionGrant, "auto-promote enabled", nil
	}
	out := k.approvals.Submit(ctx, approval.SubmitSpec{
		Capability:    "toolforge.promote",
		ToolName:      "toolforge.promote",
		Input:         fmt.Sprintf("promote %s (%s): %s", st.Name, st.Language, st.Description),
		Reason:        "agent requested promotion of forge tool " + st.Name,
		Actor:         "toolforge",
		CorrelationID: corr,
	})
	if out.Decision != approval.DecisionGrant {
		return st, out.Decision, out.Reason, nil // a verdict, not a failure
	}
	promoted, err := k.PromoteScriptTool(corr, st.Name)
	if err != nil {
		return st, out.Decision, out.Reason, err
	}
	return promoted, out.Decision, out.Reason, nil
}

// QuarantineScriptTool pulls an active tool from production — the kill
// switch. Journals scripttool.quarantined with the operator's reason.
func (k *Kernel) QuarantineScriptTool(corr, ref, reason string) (toolforge.ScriptTool, error) {
	st, err := k.toolForge.Quarantine(ref)
	if err != nil {
		return toolforge.ScriptTool{}, err
	}
	payload := map[string]any{"id": st.ID, "name": st.Name}
	if reason = strings.TrimSpace(reason); reason != "" {
		payload["reason"] = reason
	}
	_, _ = k.bus.Publish(event.Spec{
		Subject: "toolforge." + st.Name, Kind: event.KindScriptToolQuarantined, Actor: "toolforge",
		CorrelationID: corr,
		Payload:       payload,
	})
	return st, nil
}

// RemoveScriptTool deletes a script tool, journaling scripttool.removed when
// it existed. Returns whether it existed.
func (k *Kernel) RemoveScriptTool(corr, ref string) (bool, error) {
	gone, ok, err := k.toolForge.Remove(ref)
	if err != nil {
		return false, err
	}
	if ok {
		_, _ = k.bus.Publish(event.Spec{
			Subject: "toolforge." + gone.Name, Kind: event.KindScriptToolRemoved, Actor: "toolforge",
			CorrelationID: corr,
			Payload:       map[string]any{"id": gone.ID, "name": gone.Name},
		})
	}
	return ok, nil
}

// mergeScriptTools returns the run's tool map extended with the forge's
// ACTIVE scripts as callable forge_<name> tools. Registered tools always win
// a name collision (the forge_ prefix makes one effectively impossible, but
// a script must never shadow a real tool). Returns the input map untouched
// when there is nothing to offer, so the common no-scripts path stays
// allocation-free.
func (k *Kernel) mergeScriptTools(tools map[string]agent.Tool) map[string]agent.Tool {
	if k.toolForge == nil || k.cfg.ScriptRunner == nil {
		return tools
	}
	active := k.toolForge.Active()
	if len(active) == 0 {
		return tools
	}
	out := make(map[string]agent.Tool, len(tools)+len(active))
	for name, t := range tools {
		out[name] = t
	}
	for _, st := range active {
		name := forgedToolName(st.Name)
		if _, exists := out[name]; exists {
			continue
		}
		out[name] = forgedTool{st: st, runner: k.cfg.ScriptRunner}
	}
	return out
}

// forgedToolName is the callable name of a promoted script: the prefix both
// namespaces scripts away from built-in tools and lets the Edict toolmap
// route every forged call to the `code.exec` capability.
func forgedToolName(name string) string { return "forge_" + name }

// defaultForgedSchema is offered when the author supplied no input schema:
// a permissive object — the whole call input reaches the script as JSON.
const defaultForgedSchema = `{
  "type": "object",
  "additionalProperties": true
}`

// forgedTool adapts one ACTIVE script-tool record to agent.Tool: the call's
// raw JSON input rides into the sandbox (the script reads it from
// ./stdin.txt) and combined stdout+stderr comes back as the result.
type forgedTool struct {
	st     toolforge.ScriptTool
	runner toolforge.Runner
}

func (t forgedTool) Definition() agent.ToolDef {
	schema := strings.TrimSpace(t.st.InputSchema)
	if schema == "" {
		schema = defaultForgedSchema
	}
	desc := strings.TrimSpace(t.st.Description) +
		" (Forged script tool: a vetted " + t.st.Language +
		" script run in the sandbox; this call's JSON input is available to it as ./stdin.txt.)"
	return agent.ToolDef{
		Name:        forgedToolName(t.st.Name),
		Description: desc,
		InputSchema: json.RawMessage(schema),
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"Execute a promoted agent-authored script inside the configured sandbox.",
			},
			AffectedResources: []string{"code-exec sandbox", "script tool " + t.st.Name},
			RollbackNotes:     "Sandbox scratch output is disposable; any persistent or external effect depends on the sandbox profile and must be compensated manually.",
			Confidence:        0.6,
		},
	}
}

func (t forgedTool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	in := string(raw)
	if strings.TrimSpace(in) == "" {
		in = "{}"
	}
	out, isErr, err := t.runner.RunScript(ctx, t.st.Language, t.st.Code, in)
	if err != nil {
		return agent.Result{Output: forgedToolName(t.st.Name) + ": " + err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: out, IsError: isErr}, nil
}
