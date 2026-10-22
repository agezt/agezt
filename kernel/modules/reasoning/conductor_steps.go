// SPDX-License-Identifier: MIT

package reasoning

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/reasoning/api"
)

// conductorRoleModels fills any empty role with a distinct model from the
// Council's default membership (one per keyed provider), cycling if there are
// fewer than three. Errors when nothing is configured.
func (s *Service) conductorRoleModels(cfg ConductorConfig) (thinker, worker, verifier string, err error) {
	thinker = strings.TrimSpace(cfg.Thinker)
	worker = strings.TrimSpace(cfg.Worker)
	verifier = strings.TrimSpace(cfg.Verifier)
	if thinker != "" && worker != "" && verifier != "" {
		return thinker, worker, verifier, nil
	}
	var members []CouncilMember
	if s.deps.Members != nil {
		members = s.deps.Members()
	}
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
func (s *Service) conductorComplete(ctx context.Context, corr, model, system, prompt string, maxTokens int) (string, error) {
	req := llm.CompletionRequest{
		MaxTokens: maxTokens,
		System:    system,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: prompt}},
	}
	if strings.HasPrefix(model, "@") {
		req.ModelChain = []string{model}
	} else {
		req.Model = model
	}
	resp, err := s.complete(ctx, corr, "conductor", req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Message.Content), nil
}

// conductorStep runs a thinker/worker completion and records it (publishing a
// step event). Errors are carried on the step, not returned.
func (s *Service) conductorStep(ctx context.Context, corr string, round int, role, model, system, prompt string, maxTokens int) ConductorStep {
	step := ConductorStep{Round: round, Role: role, Model: model}
	text, err := s.conductorComplete(ctx, corr, model, system, prompt, maxTokens)
	if err != nil {
		step.Error = err.Error()
	} else {
		step.Text = text
	}
	s.conductorPublish(corr, event.KindConductorStep, map[string]any{
		"role": role, "model": model, "round": round,
		"text": api.Clip(step.Text, conductorEventTextMax), "error": step.Error,
	})
	return step
}

// conductorVerify runs the AUTO verifier: execute the worker's code when it
// carries a runnable fenced block and a sandbox is wired; otherwise LLM critique.
func (s *Service) conductorVerify(ctx context.Context, corr string, round int, model, brief, task, answer string) ConductorStep {
	step := ConductorStep{Round: round, Role: conductorRoleVerifier, Model: model}
	if lang, code, ok := extractRunnableCode(answer); ok && s.deps.CodeExecAvailable() && s.deps.RunCode != nil {
		res, ran, err := s.deps.RunCode(ctx, corr, "conductor-verify", lang, code, "")
		ex := &ConductorExec{Ran: ran, Language: lang, Output: api.Clip(strings.TrimSpace(res.Output), conductorEventTextMax)}
		step.Exec = ex
		switch {
		case err != nil:
			ex.OK = false
			step.Verdict = "fail"
			step.Reason = "execution error: " + err.Error()
		case res.IsError:
			ex.OK = false
			step.Verdict = "fail"
			step.Reason = "code ran but reported failure (non-zero exit / timeout); fix it"
		default:
			ex.OK = true
			step.Verdict = "pass"
			step.Reason = "code ran cleanly"
		}
	} else {
		verdict, reason := s.conductorCritique(ctx, corr, model, brief, task, answer)
		step.Verdict = verdict
		step.Reason = reason
	}
	s.conductorPublish(corr, event.KindConductorStep, map[string]any{
		"role": conductorRoleVerifier, "model": model, "round": round,
		"verdict": step.Verdict, "reason": api.Clip(step.Reason, conductorEventTextMax),
		"exec": step.Exec != nil && step.Exec.Ran,
	})
	return step
}

// conductorCritique asks the verifier model to judge the answer, returning a
// "pass"/"fail" verdict and a brief reason. A failed model call defaults to a
// pass (don't block the run on the critic's own outage), recording the reason.
func (s *Service) conductorCritique(ctx context.Context, corr, model, brief, task, answer string) (verdict, reason string) {
	prompt := fmt.Sprintf("Task:\n%s\n\nProposed answer:\n%s\n\n"+
		"Judge whether the answer correctly and completely solves the task. "+
		"Reply with PASS or FAIL on the first line, then a brief reason.", task, strings.TrimSpace(api.OrPlaceholder(answer)))
	text, err := s.conductorComplete(ctx, corr, model, conductorRoleSystem(conductorRoleVerifier, brief), prompt, conductorVerifierMaxTokens)
	if err != nil {
		return "pass", "verifier unavailable: " + err.Error()
	}
	return parseVerdict(text)
}

// conductorPlan runs the optional tailoring call. Its free-text output is stored
// verbatim on the result and (when it uses THINKER:/WORKER:/VERIFIER: sections)
// parsed into per-role briefs. A failed call yields an empty plan (the run then
// uses the static role prompts).
func (s *Service) conductorPlan(ctx context.Context, corr, task, thinker, worker, verifier string) string {
	prompt := fmt.Sprintf("You are the conductor of three LLM workers collaborating on a task.\n\n"+
		"Task:\n%s\n\nWorkers:\n- THINKER (model %s): plans the approach.\n- WORKER (model %s): writes the solution.\n"+
		"- VERIFIER (model %s): checks it.\n\nWrite ONE short focused instruction for each, tailored to get the best "+
		"out of this task. Use exactly these labels, one section each:\nTHINKER: ...\nWORKER: ...\nVERIFIER: ...",
		task, thinker, worker, verifier)
	text, err := s.conductorComplete(ctx, corr, verifier, "You are a coordination planner. Be concise and concrete.", prompt, conductorPlanMaxTokens)
	if err != nil {
		return ""
	}
	return text
}

// --- prompt builders & parsing ---

func conductorRoleSystem(role, brief string) string {
	var base string
	switch role {
	case conductorRoleThinker:
		base = "You are the Thinker. Decompose the task and lay out a clear, concrete approach the Worker can follow. Do not write the full solution — plan it."
	case conductorRoleWorker:
		base = "You are the Worker. Produce the complete solution. When the task is code, write runnable code AND include self-tests (assertions) that fail loudly if the solution is wrong, in a single fenced code block."
	case conductorRoleVerifier:
		base = "You are the Verifier. Judge strictly and concretely whether the answer solves the task."
	}
	if strings.TrimSpace(brief) != "" {
		base = base + " " + strings.TrimSpace(brief)
	}
	return base
}

func conductorThinkerPrompt(task string) string {
	return "Task:\n" + task + "\n\nGive a concrete plan/approach for solving it."
}

func conductorWorkerPrompt(task, plan, feedback string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\nThe Thinker's plan:\n%s\n", task, strings.TrimSpace(api.OrPlaceholder(plan)))
	if strings.TrimSpace(feedback) != "" {
		fmt.Fprintf(&b, "\nThe Verifier rejected your previous attempt. Fix it:\n%s\n", strings.TrimSpace(feedback))
	}
	b.WriteString("\nProduce the complete solution now.")
	return b.String()
}

// parseVerdict reads a "PASS"/"FAIL" verdict from the first non-empty line
// (tolerant of leading markdown markers), keeping any inline remainder of that
// line plus the following lines as the reason. Anything that isn't clearly PASS
// is treated as fail.
func parseVerdict(text string) (verdict, reason string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	head := ""
	idx := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		head = strings.TrimLeft(ln, " #*->\t")
		idx = i
		break
	}
	rest := ""
	if idx >= 0 {
		rest = strings.TrimSpace(strings.Join(lines[idx+1:], "\n"))
	}
	lower := strings.ToLower(head)
	if strings.HasPrefix(lower, "pass") {
		r := joinReason(strings.TrimLeft(head[len("pass"):], " :*-\t"), rest)
		if r == "" {
			r = "verifier passed the answer"
		}
		return "pass", r
	}
	inline := head
	if strings.HasPrefix(lower, "fail") {
		inline = strings.TrimLeft(head[len("fail"):], " :*-\t")
	}
	r := joinReason(inline, rest)
	if r == "" {
		r = "verifier rejected the answer"
	}
	return "fail", r
}

// joinReason combines an inline remainder with trailing lines, omitting blanks.
func joinReason(inline, rest string) string {
	inline = strings.TrimSpace(inline)
	switch {
	case inline == "":
		return rest
	case rest == "":
		return inline
	default:
		return inline + "\n" + rest
	}
}

// parseRoleBriefs splits a plan blob into per-role instructions when it uses
// THINKER:/WORKER:/VERIFIER: section labels. Missing sections are simply absent.
func parseRoleBriefs(plan string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(plan) == "" {
		return out
	}
	labels := map[string]string{
		"thinker":  conductorRoleThinker,
		"worker":   conductorRoleWorker,
		"verifier": conductorRoleVerifier,
	}
	var cur string
	var buf []string
	flush := func() {
		if cur != "" {
			out[cur] = strings.TrimSpace(strings.Join(buf, "\n"))
		}
		buf = nil
	}
	for ln := range strings.SplitSeq(plan, "\n") {
		trimmed := strings.TrimLeft(ln, " #*->\t")
		lower := strings.ToLower(trimmed)
		matched := false
		for key, role := range labels {
			if strings.HasPrefix(lower, key+":") {
				flush()
				cur = role
				buf = append(buf, strings.TrimSpace(trimmed[len(key)+1:]))
				matched = true
				break
			}
		}
		if !matched && cur != "" {
			buf = append(buf, ln)
		}
	}
	flush()
	return out
}

// conductorCodeBlock matches a fenced code block, capturing the language tag and
// the body.
var conductorCodeBlock = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\s*\\n(.*?)```")

// extractRunnableCode returns the first fenced code block whose language maps to
// a code_exec-supported runtime, normalised to that runtime's language id.
func extractRunnableCode(text string) (lang, code string, ok bool) {
	for _, m := range conductorCodeBlock.FindAllStringSubmatch(text, -1) {
		norm := normalizeExecLang(m[1])
		if norm == "" {
			continue
		}
		body := strings.TrimSpace(m[2])
		if body == "" {
			continue
		}
		return norm, body, true
	}
	return "", "", false
}

// normalizeExecLang maps a fenced-block language tag to a code_exec language id,
// or "" if it isn't a runnable language.
func normalizeExecLang(tag string) string {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "python", "py", "python3":
		return "python"
	case "javascript", "js", "node":
		return "javascript"
	case "typescript", "ts", "deno":
		return "typescript"
	default:
		return ""
	}
}
