// SPDX-License-Identifier: MIT

// Package conductor is the in-process `conductor` tool: it lets an agent run the
// Conductor (M997) — the asymmetric, verify-driven sibling of the Council. Three
// roles (Thinker, Worker, Verifier) collaborate over (usually) different models,
// the Verifier actually RUNS the worker's code when it can, and a failed check
// sends the Worker back for another round. Use it for hard, verifiable work
// (coding, math, multi-step reasoning) where a single answer isn't trustworthy.
// The orchestration lives in kernel/runtime; this tool is the thin agent-facing
// front (mirrors plugins/tools/council).
package conductor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/runtime"
)

// Runner is the slice of *runtime.Kernel the tool needs — an interface so the
// tool is decoupled and unit-testable with a fake.
type Runner interface {
	Conduct(ctx context.Context, corr string, cfg runtime.ConductorConfig) (runtime.ConductorResult, error)
}

// Tool is the `conductor` implementation of agent.Tool.
type Tool struct {
	runner Runner
}

// New returns an empty Tool; call SetRunner before use.
func New() *Tool { return &Tool{} }

// SetRunner injects the conductor orchestrator (the kernel), done by the daemon.
func (t *Tool) SetRunner(r Runner) { t.runner = r }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "conductor",
		Description: "Run the Conductor — three roles on (usually) DIFFERENT models collaborate to solve a hard, " +
			"verifiable task: a THINKER plans, a WORKER writes the solution, and a VERIFIER checks it (RUNNING the " +
			"worker's code when it can), looping until the verifier accepts or the round cap is hit. Best for coding, " +
			"math, and multi-step reasoning where one model's answer isn't enough. Returns {answer, passed, steps}. " +
			"Costs several model calls (and a sandbox run), so reserve it for tasks that warrant it.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["task"],
  "properties": {
    "task":       {"type":"string", "description":"The task to solve."},
    "thinker":    {"type":"string", "description":"Model id or @chain for the planning role (default: a keyed provider)."},
    "worker":     {"type":"string", "description":"Model id or @chain for the solving role (default: a different keyed provider)."},
    "verifier":   {"type":"string", "description":"Model id or @chain for the checking role (default: a different keyed provider)."},
    "max_rounds": {"type":"integer", "description":"Worker/verifier retry cap (default 2)."},
    "plan":       {"type":"boolean", "description":"Tailor per-role instructions with a planning call first (default false)."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"Run several model calls (and possibly a sandboxed code run) to solve and verify a task, returning the answer and transcript.",
			},
			AffectedResources: []string{"model-provider budget", "code-exec sandbox", "conductor run transcript"},
			RollbackNotes:     "No external state is changed beyond model spend, an ephemeral sandbox run, and transcript records; spend cannot be recovered.",
			Confidence:        0.8,
		},
	}
}

type input struct {
	Task      string `json:"task"`
	Thinker   string `json:"thinker,omitempty"`
	Worker    string `json:"worker,omitempty"`
	Verifier  string `json:"verifier,omitempty"`
	MaxRounds int    `json:"max_rounds,omitempty"`
	Plan      bool   `json:"plan,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("conductor: parse input: %w", err)
	}
	if t.runner == nil {
		return errResult("conductor unavailable"), nil
	}
	if strings.TrimSpace(in.Task) == "" {
		return errResult("task required"), nil
	}
	corr := agent.CorrelationFromContext(ctx)
	res, err := t.runner.Conduct(ctx, corr, runtime.ConductorConfig{
		Task:      in.Task,
		Thinker:   in.Thinker,
		Worker:    in.Worker,
		Verifier:  in.Verifier,
		MaxRounds: in.MaxRounds,
		Plan:      in.Plan,
	})
	if err != nil {
		return errResult(err.Error()), nil
	}
	out, _ := json.MarshalIndent(map[string]any{
		"answer": res.Answer,
		"passed": res.Passed,
		"roles":  res.Roles,
		"rounds": res.Rounds,
		"plan":   res.Plan,
		"steps":  res.Steps,
	}, "", "  ")
	return agent.Result{Output: string(out)}, nil
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "conductor: " + msg, IsError: true}
}
