// SPDX-License-Identifier: MIT

// Package runstool is the in-process run-introspection tool. It lets the agent
// recall its OWN past work — "have I looked into this before?", "how many runs
// have I done today and how did they go?" — by folding the daemon's own journal
// into recent top-level runs and aggregate stats (M644).
//
// This is the self-knowledge primitive that complements memory (deliberate
// facts) and world (entities): the agent can see what it has actually DONE, not
// just what it chose to remember. Read-only; sub-agent runs are folded out so
// the view is the operator-facing leads.
package runstool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// window caps how many recent journal events the tool folds — recent history is
// what matters and it bounds the scan on a long-lived daemon.
const window = 5000

// DefaultLimit / MaxLimit bound how many runs op=recent/search returns.
const (
	DefaultLimit = 10
	MaxLimit     = 50
)

// history is the journal subset the tool needs — an interface so tests inject a
// fake without a real on-disk journal.
type history interface {
	Tail(n int) ([]*event.Event, error)
}

// Tool implements agent.Tool. Created unbound via New(); Bind wires the journal.
type Tool struct {
	hist history
}

// New returns an unbound runs tool (no journal until Bind).
func New() *Tool { return &Tool{} }

// Bind wires the live journal. Called once after the kernel opens.
func (t *Tool) Bind(h history) {
	if h != nil {
		t.hist = h
	}
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "runs",
		Description: "Recall your OWN past runs from the journal: op=recent lists recent " +
			"runs (intent, status, cost, when); op=stats gives aggregate totals " +
			"(completed/failed/success-rate/spend); op=search finds past runs whose intent " +
			"matches a query. Use this to check whether you've already worked on something, " +
			"or to report on your recent activity.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":    {"type":"string", "enum":["recent","stats","search"]},
    "limit": {"type":"integer", "description":"For recent/search: max runs to return (default 10, max 50)."},
    "query": {"type":"string", "description":"For search: case-insensitive substring to match against run intents."}
  }
}`),
	}
}

type input struct {
	Op    string `json:"op"`
	Limit int    `json:"limit,omitempty"`
	Query string `json:"query,omitempty"`
}

// runRec is one folded run.
type runRec struct {
	Corr    string `json:"id"`
	Intent  string `json:"intent"`
	Status  string `json:"status"` // running | completed | failed
	Reason  string `json:"reason,omitempty"`
	SpentMC int64  `json:"spent_microcents"`
	TSMS    int64  `json:"last_ts_unix_ms"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("runs: parse input: %w", err)
	}
	if t.hist == nil {
		return errResult("run history is not available on this daemon"), nil
	}
	evs, err := t.hist.Tail(window)
	if err != nil {
		return errResult("read journal: " + err.Error()), nil
	}
	runs := fold(evs)
	// Newest activity first.
	sort.Slice(runs, func(i, j int) bool { return runs[i].TSMS > runs[j].TSMS })

	switch in.Op {
	case "recent":
		return okJSON(map[string]any{"runs": clip(runs, limitOf(in.Limit))}), nil
	case "search":
		q := strings.ToLower(strings.TrimSpace(in.Query))
		if q == "" {
			return errResult(`op=search needs a "query"`), nil
		}
		var hits []runRec
		for _, r := range runs {
			if strings.Contains(strings.ToLower(r.Intent), q) {
				hits = append(hits, r)
			}
		}
		return okJSON(map[string]any{"query": in.Query, "matches": len(hits), "runs": clip(hits, limitOf(in.Limit))}), nil
	case "stats":
		return okJSON(statsOf(runs)), nil
	case "":
		return errResult("op required (recent|stats|search)"), nil
	default:
		return errResult("unknown op " + in.Op + " (recent|stats|search)"), nil
	}
}

// fold reduces journal events into top-level runs, excluding sub-agent runs (a
// sub-agent's correlation appears as a subagent.spawned child_correlation).
func fold(evs []*event.Event) []runRec {
	subAgents := map[string]bool{}
	for _, e := range evs {
		if e.Kind == event.KindSubAgentSpawned {
			var p struct {
				Child string `json:"child_correlation"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			if p.Child != "" {
				subAgents[p.Child] = true
			}
		}
	}

	byCorr := map[string]*runRec{}
	get := func(corr string) *runRec {
		r := byCorr[corr]
		if r == nil {
			r = &runRec{Corr: corr, Status: "running"}
			byCorr[corr] = r
		}
		return r
	}
	for _, e := range evs {
		if e.CorrelationID == "" {
			continue
		}
		switch e.Kind {
		case event.KindTaskReceived:
			var p struct {
				Intent string `json:"intent"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			r := get(e.CorrelationID)
			r.Intent = p.Intent
			if e.TSUnixMS > r.TSMS {
				r.TSMS = e.TSUnixMS
			}
		case event.KindTaskCompleted:
			r := get(e.CorrelationID)
			r.Status = "completed"
			if e.TSUnixMS > r.TSMS {
				r.TSMS = e.TSUnixMS
			}
		case event.KindTaskFailed:
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			r := get(e.CorrelationID)
			r.Status = "failed"
			r.Reason = p.Reason
			if e.TSUnixMS > r.TSMS {
				r.TSMS = e.TSUnixMS
			}
		case event.KindBudgetConsumed:
			var p struct {
				CostMC int64 `json:"cost_microcents"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			get(e.CorrelationID).SpentMC += p.CostMC
		}
	}

	out := make([]runRec, 0, len(byCorr))
	for corr, r := range byCorr {
		if subAgents[corr] || r.Intent == "" {
			continue // skip sub-agents and entries that never saw a task.received
		}
		out = append(out, *r)
	}
	return out
}

func statsOf(runs []runRec) map[string]any {
	var completed, failed, running int
	var spent int64
	for _, r := range runs {
		switch r.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		default:
			running++
		}
		spent += r.SpentMC
	}
	total := len(runs)
	rate := 0.0
	if done := completed + failed; done > 0 {
		rate = float64(completed) / float64(done)
	}
	return map[string]any{
		"total": total, "completed": completed, "failed": failed, "running": running,
		"success_rate": rate, "total_spent_microcents": spent,
	}
}

func limitOf(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

func clip(runs []runRec, n int) []runRec {
	if runs == nil {
		return []runRec{}
	}
	if len(runs) > n {
		return runs[:n]
	}
	return runs
}

func okJSON(v any) agent.Result {
	enc, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "runs: " + msg, IsError: true}
}
