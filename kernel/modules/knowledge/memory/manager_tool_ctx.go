// SPDX-License-Identifier: MIT

package memory

// Provenance: Package memory: context.Context readers for the memory tool's
//             correlation + scope threading (CorrelationFrom + ScopeFrom).
//             Extracted from manager_tool.go during the Day-211 god-file split.
//
// The SEAM moved to the CONTRACT (W3.57): the runtime sets the correlation and
// the scope on a run's context, and the module's tool reads them — so the
// layers above the module would have to name this module to set them. The
// keys live in the contract with the helpers: a context key is an identity,
// and two packages that each define one would not see each other's values.
//
// The SETTERS are the contract's alone now: the layers above the module write
// the seam, and they may not name the module to do it. The module only READS
// it — so it keeps the readers and no longer re-exports the writers.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
)

// CorrelationFrom extracts the correlation id set by WithCorrelation.
func CorrelationFrom(ctx context.Context) string {
	return memoryapi.CorrelationFrom(ctx)
}

// ScopeFrom extracts the per-agent memory scope set by WithScope ("" = none).
func ScopeFrom(ctx context.Context) string {
	return memoryapi.ScopeFrom(ctx)
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
