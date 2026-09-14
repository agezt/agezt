// SPDX-License-Identifier: MIT

// Package memory: small tool helpers — toolActor (extract the caller's
// agent slug from context) + toolTags (the per-call tag set) + scopeOf
// (read the scope tag) + filterScope (scope filter on Records) +
// renderHits (markdown table renderer for scored hits) + plural (n-things
// pluraliser) + distillResult (the LLM-distilled record payload). Extracted
// from manager_tool.go during the Day-211 god-file split. Public API
// unchanged.
package memory


import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

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
