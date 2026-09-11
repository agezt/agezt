// SPDX-License-Identifier: MIT

// Memory tool surface: correlation/scope context helpers + Tool/Definition/Invoke + toolActor/toolTags/scopeOf/filterScope/renderHits/plural helpers.
// Code extracted from manager.go during the Day-44 god-file split. Public API unchanged.
package memory


import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)


func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

const ctxKeyScope ctxKey = iota + 1

// WithScope returns a child context carrying the run's per-agent memory scope
// (M786): when a run executes AS a named agent (M783), its recalls — the
// context injection and the memory tool — default to this scope, so the agent
// sees its own private notes on top of shared memory without having to name
// itself. Writes default to this scope too (M915 — each agent keeps its own
// memory; the shared brain is opt-in via the tool's shared=true and kept
// selective). The explicit tool scope param always wins over this default.
func WithScope(ctx context.Context, scope string) context.Context {
	if scope == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyScope, scope)
}

// ScopeFrom extracts the per-agent memory scope set by WithScope ("" = none).
func ScopeFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyScope).(string); ok {
		return v
	}
	return ""
}

// --- agent tool -----------------------------------------------------------

// toolInputSchema is the JSON Schema advertised to the model for the
// in-process `memory` tool.
const toolInputSchema = `{
  "type": "object",
  "properties": {
    "action":  {"type": "string", "enum": ["remember", "recall", "forget", "find_related", "bulk_forget"], "description": "what to do"},
    "subject": {"type": "string", "description": "entity/topic the memory is about (remember)"},
    "content": {"type": "string", "description": "the text to remember (remember)"},
    "type":    {"type": "string", "enum": ["FACT","SUMMARY","RELATION","PREFERENCE","OBSERVATION"], "description": "memory type (remember; default FACT)"},
    "evidence":{"type": "string", "enum": ["observed","inferred","curated","constraint"], "description": "epistemic source class (remember; default inferred/derived from source)"},
    "half_life_ms":{"type": "integer", "description": "mechanical expiration budget in milliseconds (remember; default by evidence/type)"},
    "query":   {"type": "string", "description": "search text (recall)"},
    "limit":   {"type": "integer", "description": "max results (recall; default 5)"},
    "id":      {"type": "string", "description": "record id (forget, find_related)"},
    "ids":     {"type": "array", "items": {"type": "string"}, "description": "record ids (bulk_forget)"},
    "shared":  {"type": "boolean", "description": "remember only: write to the SHARED memory every agent recalls. Be selective — share only durable facts useful to ALL agents (owner preferences, project-wide decisions). Default false: the note stays private to you."},
    "scope":   {"type": "string", "description": "optional namespace override, e.g. a role like \"researcher\". On remember: store the note private to that scope (default: your own agent scope). On recall: also surface that scope's private notes. Shared memory is ALWAYS visible; another scope's private notes never are."}
  },
  "required": ["action"]
}`

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

// toolActor resolves who an agent's memory write should be attributed to (M851):
// the named roster agent's slug when the run executes AS one, else the generic
// "agent" (a default-identity run). Operator (console/CLI) and distilled writes
// set their own actor at their call sites.
func toolActor(ctx context.Context) string {
	if slug := agent.AgentFromContext(ctx); slug != "" {
		return slug
	}
	return "agent"
}

// toolTags builds a tool write's tag map. Tool writes are tagged source=agent so
// they are distinguishable from operator and distilled writes; a non-empty scope
// tag makes the note private to that namespace (recall only surfaces it when the
// same scope is requested) — the per-agent layer over shared memory (M652/M915).
func toolTags(scope string) map[string]string {
	t := map[string]string{"source": "agent"}
	if scope != "" {
		t["scope"] = scope
	}
	return t
}

// scopeOf extracts the scope tag from a record's tag map ("" = shared).
func scopeOf(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	return tags["scope"]
}

// filterScope drops records private to a scope other than the requested one.
// A record is visible when it carries no scope tag (shared) or its scope equals
// the caller's. Returns a new slice; the input is not mutated.
func filterScope(recs []Record, scope string) []Record {
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		rs := ""
		if r.Tags != nil {
			rs = r.Tags["scope"]
		}
		if rs == "" || rs == scope {
			out = append(out, r)
		}
	}
	return out
}

func renderHits(hits []Scored) string {
	if len(hits) == 0 {
		return "no relevant memory found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d relevant memor%s:\n", len(hits), plural(len(hits)))
	for _, h := range hits {
		scope := ""
		if h.Record.Tags != nil && h.Record.Tags["scope"] != "" {
			scope = " (scope: " + h.Record.Tags["scope"] + ")"
		}
		fmt.Fprintf(&b, "- [%s] %s: %s%s\n", h.Record.Type, h.Record.Subject, h.Record.Content, scope)
	}
	return strings.TrimRight(b.String(), "\n")
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// --- distillation ---------------------------------------------------------

// distillSystem instructs the provider to extract durable, reusable facts
// from a completed task. The model must return a JSON object so parsing is
// deterministic; any non-JSON or empty response yields zero facts (the
// best-effort contract — distillation never fails a task).
const distillSystem = `You review a completed agent task and extract durable, reusable facts worth remembering for future tasks. ` +
	`Return ONLY a JSON object of the form {"facts":[{"subject":"...","content":"...","type":"FACT|SUMMARY|PREFERENCE"}]}. ` +
	`Extract at most 3 facts. Prefer specific, durable knowledge (project structure, decisions, user preferences) over transient details. ` +
	`If nothing is worth remembering, return {"facts":[]}.`

type distillResult struct {
	Facts []struct {
		Subject string `json:"subject"`
		Content string `json:"content"`
		Type    Type   `json:"type"`
	} `json:"facts"`
}

// Distill runs one best-effort LLM call over a task transcript and stores any
// extracted facts (tagged source=distill) under corr. It returns the ids it
// created. Errors are returned for the caller to journal, but the caller must
// never let a distillation error fail the underlying task.