// SPDX-License-Identifier: MIT

// Package memory: agent.Tool surface for the memory manager (toolInput +
// memoryTool + Tool + Definition + Invoke). The context.Context helpers
// (WithCorrelation + CorrelationFrom + WithScope + ScopeFrom) moved to
// manager_tool_ctx.go; the small tool helpers (toolActor + toolTags +
// scopeOf + filterScope + renderHits + plural + distillResult) moved to
// manager_tool_helpers.go. Day-211 god-file split. Public API unchanged.
package memory


import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)

type toolInput struct {
	Action     string   `json:"action"`
	Subject    string   `json:"subject"`
	Content    string   `json:"content"`
	Type       Type     `json:"type"`
	Evidence   Evidence `json:"evidence"`
	HalfLifeMS int64    `json:"half_life_ms"`
	Query      string   `json:"query"`
	Limit      int      `json:"limit"`
	ID         string   `json:"id"`
	IDs        []string `json:"ids"`
	Shared     bool     `json:"shared"`
	Scope      string   `json:"scope"`
}

// memoryTool is the in-process agent.Tool that lets the agent remember,
// recall, and forget during a run. Writes are journaled by the Manager under
// the run's correlation (read from ctx via CorrelationFrom).
type memoryTool struct{ mgr *Manager }

// Tool returns the agent-facing memory tool. Register it under the name
// "memory" in the agent loop's tool map.
func (m *Manager) Tool() agent.Tool { return memoryTool{mgr: m} }

func (t memoryTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:       "memory",
		Capability: agent.ToolCapability{Name: string(edict.CapMemory)},
		Description: "Persist and retrieve durable knowledge across tasks. " +
			"action=remember stores a fact (subject, content); " +
			"action=recall searches stored memory (query); " +
			"action=forget tombstones a record (id). " +
			"Your notes are PRIVATE to you by default — recall surfaces them plus the shared memory. " +
			"Pass shared=true only for facts genuinely useful to ALL agents " +
			"(owner preferences, project-wide decisions); be selective about the shared brain.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"read durable memory for recall/find_related actions",
				"write or tombstone durable memory for remember/forget/bulk_forget actions",
			},
			AffectedResources: []string{"memory store", "agent/private memory scope", "shared memory scope when requested"},
			RollbackNotes:     "Recall needs no rollback. Remembered records can be tombstoned with forget; tombstones and writes are journaled for audit/replay.",
			Confidence:        0.9,
		},
		InputSchema: json.RawMessage(toolInputSchema),
	}
}

func (t memoryTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid memory input: " + err.Error(), IsError: true}, nil
	}
	corr := CorrelationFrom(ctx)
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "remember":
		// Private-by-default (M915): a named agent's write lands in its OWN
		// scope unless it explicitly opts into the shared brain (shared=true,
		// or the "shared" scope sentinel a model may plausibly produce). An
		// explicit scope param still wins; an unscoped run (no agent identity)
		// keeps writing shared, as before.
		scope := strings.TrimSpace(in.Scope)
		switch {
		case in.Shared || strings.EqualFold(scope, "shared"):
			scope = ""
		case scope == "":
			scope = ScopeFrom(ctx)
		}
		rec, created, err := t.mgr.Remember(corr, RememberSpec{
			Type: in.Type, Subject: in.Subject, Content: in.Content, Tags: toolTags(scope),
			Evidence: in.Evidence, HalfLifeMS: in.HalfLifeMS,
			Actor: toolActor(ctx), // who is writing — the agent slug, or "agent" (M851)
		})
		if err != nil {
			return agent.Result{Output: "remember failed: " + err.Error(), IsError: true}, nil
		}
		verb := "reinforced"
		if created {
			verb = "stored"
		}
		where := "shared"
		if scope != "" {
			where = "private to " + scope
		}
		return agent.Result{Output: fmt.Sprintf("%s memory %s (%s: %s) — %s", verb, rec.ID[:12], rec.Type, rec.Subject, where)}, nil
	case "recall":
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		// The run's per-agent scope (M786) is the DEFAULT visibility: a named
		// agent recalls its own private notes + shared memory without naming
		// itself. An explicit scope param wins (e.g. peeking at a teammate's
		// scope is still expressible — records stay readable, never hidden
		// behind identity).
		scope := strings.TrimSpace(in.Scope)
		if scope == "" {
			scope = ScopeFrom(ctx)
		}
		hits, err := t.mgr.RecallScoped(corr, in.Query, limit, scope)
		if err != nil {
			return agent.Result{Output: "recall failed: " + err.Error(), IsError: true}, nil
		}
		return agent.Result{Output: renderHits(hits)}, nil
	case "forget":
		ok, err := t.mgr.Forget(corr, in.ID)
		if err != nil {
			return agent.Result{Output: "forget failed: " + err.Error(), IsError: true}, nil
		}
		if !ok {
			return agent.Result{Output: "no memory with id " + in.ID, IsError: true}, nil
		}
		return agent.Result{Output: "forgot memory " + in.ID}, nil
	case "find_related":
		if in.ID == "" {
			return agent.Result{Output: "find_related requires id", IsError: true}, nil
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		if limit > 100 {
			limit = 100
		}
		seed, found, err := t.mgr.Get(in.ID)
		if err != nil {
			return agent.Result{Output: "find_related failed: " + err.Error(), IsError: true}, nil
		}
		if !found {
			return agent.Result{Output: "seed memory id " + in.ID + " not found", IsError: true}, nil
		}
		hits, err := t.mgr.Search(seed.Content, limit+1) // +1 because seed itself may appear
		if err != nil {
			return agent.Result{Output: "find_related failed: " + err.Error(), IsError: true}, nil
		}
		// Exclude the seed record from results.
		out := make([]string, 0, limit)
		for _, h := range hits {
			if h.Record.ID != in.ID {
				out = append(out, fmt.Sprintf("[%.3f] %s (%s: %s)", h.Score, h.Record.Subject, h.Record.ID[:12], h.Record.Type))
			}
			if len(out) >= limit {
				break
			}
		}
		if len(out) == 0 {
			return agent.Result{Output: "no related memories found for " + in.ID}, nil
		}
		return agent.Result{Output: "related memories for " + in.ID + ":\n" + strings.Join(out, "\n")}, nil
	case "bulk_forget":
		if len(in.IDs) == 0 {
			return agent.Result{Output: "bulk_forget requires ids", IsError: true}, nil
		}
		if len(in.IDs) > 500 {
			return agent.Result{Output: "bulk_forget exceeds 500 ids per call", IsError: true}, nil
		}
		var forgotten, notFound int
		for _, id := range in.IDs {
			ok, err := t.mgr.Forget(corr, id)
			if err != nil {
				return agent.Result{Output: "bulk_forget failed: " + err.Error(), IsError: true}, nil
			}
			if ok {
				forgotten++
			} else {
				notFound++
			}
		}
		return agent.Result{Output: fmt.Sprintf("forgotten: %d  not_found: %d", forgotten, notFound)}, nil
	default:
		return agent.Result{Output: "unknown action " + in.Action + " (remember|recall|forget|find_related|bulk_forget)", IsError: true}, nil
	}
}
