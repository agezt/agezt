// SPDX-License-Identifier: MIT

// Package memory: context.Context helpers for the memory tool's correlation
// + scope threading (WithCorrelation + CorrelationFrom + WithScope +
// ScopeFrom). Extracted from manager_tool.go during the Day-211 god-file
// split. Public API unchanged.
package memory


import (
	"context"
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

