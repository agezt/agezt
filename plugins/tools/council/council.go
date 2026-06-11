// SPDX-License-Identifier: MIT

// Package council is the in-process `council` tool: it lets an agent convene the
// Council of Elders (M837) — a panel of differently-modelled advisors that
// deliberates a question and returns a consensus. So in any situation an agent
// can defer a hard or high-stakes decision to several strong models and get back
// a reconciled answer ("herhangi bir durumda bu heyete başvurulup buradan cevap
// alınabilir"). The orchestration lives in kernel/runtime; this tool is the thin
// agent-facing front.
package council

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
	Council(ctx context.Context, corr, question string, members []runtime.CouncilMember, rounds int) (runtime.CouncilResult, error)
}

// Tool is the `council` implementation of agent.Tool.
type Tool struct {
	runner Runner
}

// New returns an empty Tool; call SetRunner before use.
func New() *Tool { return &Tool{} }

// SetRunner injects the council orchestrator (the kernel), done by the daemon.
func (t *Tool) SetRunner(r Runner) { t.runner = r }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "council",
		Description: "Convene the Council of Elders — a panel of several advisors, each on a DIFFERENT " +
			"model, that debate your question and return a CONSENSUS (plus any dissent). Use it for hard, " +
			"high-stakes, or contested decisions where one model's answer isn't enough. Returns " +
			"{consensus, dissent, opinions}. Costs several model calls, so reserve it for questions that " +
			"warrant a panel.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["question"],
  "properties": {
    "question": {"type":"string", "description":"The question to put before the council."},
    "rounds":   {"type":"integer", "description":"Deliberation rounds after the opening positions (default 1)."}
  }
}`),
	}
}

type input struct {
	Question string `json:"question"`
	Rounds   int    `json:"rounds,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("council: parse input: %w", err)
	}
	if t.runner == nil {
		return errResult("council unavailable"), nil
	}
	if strings.TrimSpace(in.Question) == "" {
		return errResult("question required"), nil
	}
	corr := agent.CorrelationFromContext(ctx)
	res, err := t.runner.Council(ctx, corr, in.Question, nil, in.Rounds)
	if err != nil {
		return errResult(err.Error()), nil
	}
	opinions := make([]map[string]any, 0, len(res.Opinions))
	for _, op := range res.Opinions {
		row := map[string]any{"seat": op.Seat, "model": op.Model, "round": op.Round, "text": op.Text}
		if op.Error != "" {
			row["error"] = op.Error
		}
		opinions = append(opinions, row)
	}
	out, _ := json.MarshalIndent(map[string]any{
		"consensus": res.Consensus,
		"dissent":   res.Dissent,
		"members":   res.Members,
		"rounds":    res.Rounds,
		"opinions":  opinions,
	}, "", "  ")
	return agent.Result{Output: string(out)}, nil
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "council: " + msg, IsError: true}
}
