// SPDX-License-Identifier: MIT

// Conductor steps: conductorRoleModels + conductorComplete + conductorStep + conductorVerify + conductorCritique + conductorPlan + conductorPublish.
// Code extracted from conductor.go during the Day-76 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"strings"
)


func (k *Kernel) conductorRoleModels(cfg ConductorConfig) (thinker, worker, verifier string, err error) {
	thinker = strings.TrimSpace(cfg.Thinker)
	worker = strings.TrimSpace(cfg.Worker)
	verifier = strings.TrimSpace(cfg.Verifier)
	if thinker != "" && worker != "" && verifier != "" {
		return thinker, worker, verifier, nil
	}
	members := k.CouncilDefaultMembers()
	pick := func(i int) string {
		if len(members) == 0 {
			return ""
		}
		return members[i%len(members)].Model
	}
	if thinker == "" {
		thinker = pick(0)
	}
	if worker == "" {
		worker = pick(1)
	}
	if verifier == "" {
		verifier = pick(2)
	}
	if thinker == "" || worker == "" || verifier == "" {
		return "", "", "", fmt.Errorf("conductor: no models available (set thinker/worker/verifier, or configure keyed providers)")
	}
	return thinker, worker, verifier, nil
}

// conductorComplete runs one role completion, routing a bare model id directly or
// a "@chain" reference through the Governor's chain expansion (via ModelChain).
func (k *Kernel) conductorComplete(ctx context.Context, corr, model, system, prompt string, maxTokens int) (string, error) {
	req := agent.CompletionRequest{
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	}
	if strings.HasPrefix(model, "@") {
		req.ModelChain = []string{model}
	} else {
		req.Model = model
	}
	resp, err := k.completeAux(ctx, corr, "conductor", req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Message.Content), nil
}

// conductorStep runs a thinker/worker completion and records it (publishing a
// step event). Errors are carried on the step, not returned.
func (k *Kernel) conductorStep(ctx context.Context, corr string, round int, role, model, system, prompt string, maxTokens int) ConductorStep {
	step := ConductorStep{Round: round, Role: role, Model: model}
	text, err := k.conductorComplete(ctx, corr, model, system, prompt, maxTokens)
	if err != nil {
		step.Error = err.Error()
	} else {
		step.Text = text
	}
	k.conductorPublish(corr, event.KindConductorStep, map[string]any{
		"role": role, "model": model, "round": round,
		"text": clip(step.Text, conductorEventTextMax), "error": step.Error,
	})
	return step
}

// conductorVerify runs the AUTO verifier: execute the worker's code when it
// carries a runnable fenced block and a sandbox is wired; otherwise LLM critique.
func (k *Kernel) conductorVerify(ctx context.Context, corr string, round int, model, brief, task, answer string) ConductorStep {
	step := ConductorStep{Round: round, Role: conductorRoleVerifier, Model: model}
	if lang, code, ok := extractRunnableCode(answer); ok && k.conductorExec != nil {
		out, isErr, err := k.conductorExec.RunScript(ctx, lang, code, "")
		ex := &ConductorExec{Ran: true, Language: lang, Output: clip(strings.TrimSpace(out), conductorEventTextMax)}
		step.Exec = ex
		switch {
		case err != nil:
			ex.OK = false
			step.Verdict = "fail"
			step.Reason = "execution error: " + err.Error()
		case isErr:
			ex.OK = false
			step.Verdict = "fail"
			step.Reason = "code ran but reported failure (non-zero exit / timeout); fix it"
		default:
			ex.OK = true
			step.Verdict = "pass"
			step.Reason = "code ran cleanly"
		}
	} else {
		verdict, reason := k.conductorCritique(ctx, corr, model, brief, task, answer)
		step.Verdict = verdict
		step.Reason = reason
	}
	k.conductorPublish(corr, event.KindConductorStep, map[string]any{
		"role": conductorRoleVerifier, "model": model, "round": round,
		"verdict": step.Verdict, "reason": clip(step.Reason, conductorEventTextMax),
		"exec": step.Exec != nil,
	})
	return step
}

// conductorCritique asks the verifier model to judge the answer, returning a
// "pass"/"fail" verdict and a brief reason. A failed model call defaults to a
// pass (don't block the run on the critic's own outage), recording the reason.
func (k *Kernel) conductorCritique(ctx context.Context, corr, model, brief, task, answer string) (verdict, reason string) {
	prompt := fmt.Sprintf("Task:\n%s\n\nProposed answer:\n%s\n\n"+
		"Judge whether the answer correctly and completely solves the task. "+
		"Reply with PASS or FAIL on the first line, then a brief reason.", task, strings.TrimSpace(orPlaceholder(answer)))
	text, err := k.conductorComplete(ctx, corr, model, conductorRoleSystem(conductorRoleVerifier, brief), prompt, conductorVerifierMaxTokens)
	if err != nil {
		return "pass", "verifier unavailable: " + err.Error()
	}
	return parseVerdict(text)
}

// conductorPlan runs the optional tailoring call. Its free-text output is stored
// verbatim on the result and (when it uses THINKER:/WORKER:/VERIFIER: sections)
// parsed into per-role briefs. A failed call yields an empty plan (the run then
// uses the static role prompts).
func (k *Kernel) conductorPlan(ctx context.Context, corr, task, thinker, worker, verifier string) string {
	prompt := fmt.Sprintf("You are the conductor of three LLM workers collaborating on a task.\n\n"+
		"Task:\n%s\n\nWorkers:\n- THINKER (model %s): plans the approach.\n- WORKER (model %s): writes the solution.\n"+
		"- VERIFIER (model %s): checks it.\n\nWrite ONE short focused instruction for each, tailored to get the best "+
		"out of this task. Use exactly these labels, one section each:\nTHINKER: ...\nWORKER: ...\nVERIFIER: ...",
		task, thinker, worker, verifier)
	text, err := k.conductorComplete(ctx, corr, verifier, "You are a coordination planner. Be concise and concrete.", prompt, conductorPlanMaxTokens)
	if err != nil {
		return ""
	}
	return text
}

func (k *Kernel) conductorPublish(corr string, kind event.Kind, payload map[string]any) {
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "conductor." + corr,
		Kind:          kind,
		Actor:         "conductor",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// --- prompt builders & parsing ---