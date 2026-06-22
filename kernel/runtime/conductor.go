// SPDX-License-Identifier: MIT

package runtime

// Conductor (M997): the asymmetric, verify-driven sibling of the Council. Where
// the Council convenes equal advisors and reconciles a consensus, the Conductor
// assigns three DISTINCT roles across (usually different) models and loops until
// a verifier accepts the answer:
//
//	Thinker  — decomposes the task into an approach/plan.
//	Worker   — produces the solution from that plan (and, on a retry, the
//	           verifier's feedback).
//	Verifier — checks the worker's answer. AUTO: if the answer carries a fenced
//	           code block in a supported language and a sandbox is wired, it
//	           RUNS the code (conductorExec); otherwise it critiques with an LLM.
//
// A FAIL sends the Worker back for another round (bounded by MaxRounds) — the
// practical, training-free form of "test-time scaling" the Trinity/Conductor
// papers describe. Role→model selection is operator-owned (a bare model id or a
// "@chain" reference the Governor expands), so this never reintroduces the
// "model picks its own model" footgun (DECISIONS C2); the optional Plan call
// only TAILORS instructions and its output is recorded for audit.
//
// Built on the same one-shot Governor completion the Council uses
// (k.cfg.Provider.Complete with per-request model routing); this file just
// orchestrates the roles, the verifier branch, and the retry loop.

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// CodeExecutor is the minimal slice of the code_exec tool the Verifier needs to
// actually run a worker's code. Satisfied by *codeexec.Tool (its RunScript), but
// declared here so the kernel never imports the plugin (jarvis-pillars-map: the
// kernel imports no plugins). Wired by the daemon via SetConductorExec.
type CodeExecutor interface {
	// RunScript runs code once in the sandbox and returns its combined output,
	// an isError verdict (non-zero exit / timeout / unavailable language), and a
	// transport error. inputJSON is surfaced to the script as ./stdin.txt.
	RunScript(ctx context.Context, language, code, inputJSON string) (output string, isError bool, err error)
}

// SetConductorExec injects the code-execution backend used by the Conductor's
// Verifier role. Called once after Open (before any run), mirroring the council
// SetRunner / code_exec Bind wiring. Passing nil disables exec-verification (the
// Verifier then always critiques).
func (k *Kernel) SetConductorExec(x CodeExecutor) { k.conductorExec = x }

// Conductor role labels (also used as event/result discriminators).
const (
	conductorRoleThinker  = "thinker"
	conductorRoleWorker   = "worker"
	conductorRoleVerifier = "verifier"
)

const (
	// conductorDefaultRounds is the default worker↔verifier retry cap: one
	// initial attempt plus one retry. Enough to recover from a single bad draft
	// without burning many model calls.
	conductorDefaultRounds = 2
	// Per-role token bounds — Worker is the largest (it writes the solution).
	conductorThinkerMaxTokens  = 900
	conductorWorkerMaxTokens   = 1600
	conductorVerifierMaxTokens = 600
	conductorPlanMaxTokens     = 700
	// conductorEventTextMax clips text carried in bus events (mirrors the
	// council's councilEventTextMax) so the live Web UI can render a run without
	// re-fetching while keeping the hash-chained journal from bloating.
	conductorEventTextMax = 4000
)

// ConductorConfig parameterises one Conduct run.
type ConductorConfig struct {
	Task      string
	Thinker   string // model id or "@chain"; empty → filled from default members
	Worker    string
	Verifier  string
	MaxRounds int  // worker↔verifier retry cap; <=0 → conductorDefaultRounds
	Plan      bool // run the optional instruction-tailoring call first
}

// ConductorStep is one recorded action in the transcript.
type ConductorStep struct {
	Round   int            `json:"round"`
	Role    string         `json:"role"` // thinker|worker|verifier
	Model   string         `json:"model"`
	Text    string         `json:"text,omitempty"`
	Verdict string         `json:"verdict,omitempty"` // verifier: pass|fail
	Reason  string         `json:"reason,omitempty"`
	Exec    *ConductorExec `json:"exec,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// ConductorExec records a verifier code-execution.
type ConductorExec struct {
	Ran      bool   `json:"ran"`
	OK       bool   `json:"ok"`
	Language string `json:"language,omitempty"`
	Output   string `json:"output,omitempty"`
}

// ConductorResult is the outcome of a Conduct run.
type ConductorResult struct {
	Task   string            `json:"task"`
	Roles  map[string]string `json:"roles"` // role→resolved model label
	Plan   string            `json:"plan,omitempty"`
	Rounds int               `json:"rounds"` // worker attempts made
	Steps  []ConductorStep   `json:"steps"`
	Answer string            `json:"answer"`
	Passed bool              `json:"passed"`
}

// Conduct runs the Thinker→Worker→Verifier loop on a task. Empty role models are
// filled from the Council's default membership (one model per keyed provider) so
// the roles run on DIFFERENT models out of the box. rounds<=0 uses
// conductorDefaultRounds. A role completion failing is recorded on its step
// (Error) rather than aborting the run; only an empty task or no available
// models errors.
func (k *Kernel) Conduct(ctx context.Context, corr string, cfg ConductorConfig) (ConductorResult, error) {
	cfg.Task = strings.TrimSpace(cfg.Task)
	if cfg.Task == "" {
		return ConductorResult{}, fmt.Errorf("conductor: task required")
	}
	thinker, worker, verifier, err := k.conductorRoleModels(cfg)
	if err != nil {
		return ConductorResult{}, err
	}
	rounds := cfg.MaxRounds
	if rounds <= 0 {
		rounds = conductorDefaultRounds
	}

	result := ConductorResult{
		Task:  cfg.Task,
		Roles: map[string]string{conductorRoleThinker: thinker, conductorRoleWorker: worker, conductorRoleVerifier: verifier},
	}

	k.conductorPublish(corr, event.KindConductorStarted, map[string]any{
		"task":     clip(cfg.Task, 500),
		"thinker":  thinker,
		"worker":   worker,
		"verifier": verifier,
		"rounds":   rounds,
		"plan":     cfg.Plan,
	})

	// Optional Plan: tailor per-role instructions. Stored on the result so the
	// "what coordination did the conductor choose" decision stays auditable.
	briefs := map[string]string{}
	if cfg.Plan {
		result.Plan = k.conductorPlan(ctx, corr, cfg.Task, thinker, worker, verifier)
		briefs = parseRoleBriefs(result.Plan)
	}

	// Thinker: one decomposition pass.
	thinkStep := k.conductorStep(ctx, corr, 0, conductorRoleThinker, thinker,
		conductorRoleSystem(conductorRoleThinker, briefs[conductorRoleThinker]),
		conductorThinkerPrompt(cfg.Task), conductorThinkerMaxTokens)
	result.Steps = append(result.Steps, thinkStep)
	plan := thinkStep.Text

	// Worker↔Verifier loop.
	var feedback string
	for attempt := 1; attempt <= rounds; attempt++ {
		result.Rounds = attempt
		workStep := k.conductorStep(ctx, corr, attempt, conductorRoleWorker, worker,
			conductorRoleSystem(conductorRoleWorker, briefs[conductorRoleWorker]),
			conductorWorkerPrompt(cfg.Task, plan, feedback), conductorWorkerMaxTokens)
		result.Steps = append(result.Steps, workStep)
		result.Answer = workStep.Text

		verStep := k.conductorVerify(ctx, corr, attempt, verifier, briefs[conductorRoleVerifier], cfg.Task, workStep.Text)
		result.Steps = append(result.Steps, verStep)
		if verStep.Verdict == "pass" {
			result.Passed = true
			break
		}
		feedback = verStep.Reason
		if verStep.Exec != nil && !verStep.Exec.OK && verStep.Exec.Output != "" {
			feedback = strings.TrimSpace(feedback + "\n\nExecution output:\n" + verStep.Exec.Output)
		}
	}

	k.conductorPublish(corr, event.KindConductorDone, map[string]any{
		"passed": result.Passed,
		"rounds": result.Rounds,
		"answer": clip(result.Answer, conductorEventTextMax),
	})
	return result, nil
}

// conductorRoleModels fills any empty role with a distinct model from the
// Council's default membership (one per keyed provider), cycling if there are
// fewer than three. Errors when nothing is configured.
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
		CorrelationID: corr,
		TaskType:      "conductor",
		MaxTokens:     maxTokens,
		System:        system,
		Messages:      []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	}
	if strings.HasPrefix(model, "@") {
		req.ModelChain = []string{model}
	} else {
		req.Model = model
	}
	resp, err := k.cfg.Provider.Complete(ctx, req)
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
	fmt.Fprintf(&b, "Task:\n%s\n\nThe Thinker's plan:\n%s\n", task, strings.TrimSpace(orPlaceholder(plan)))
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
