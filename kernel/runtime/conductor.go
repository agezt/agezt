// SPDX-License-Identifier: MIT

// Conductor core: types + SetConductorExec + Conduct main entry.
// Code extracted from conductor.go during the Day-76 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"strings"
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