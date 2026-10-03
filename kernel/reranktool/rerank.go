// SPDX-License-Identifier: MIT

// Package reranktool provides the agent-facing rerank tool (M997): it lets a
// running agent reorder a set of candidate documents by relevance to a query,
// using a dedicated reranking model (more accurate than embedding cosine for
// final ranking). It drives the daemon-injected rerank adapter
// (runtime.Config.Reranker — typically the Cohere/Jina-style rerank plugin).
// The kernel never imports the plugin — only this interface.
package reranktool

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
)

// Reranker is the seam the `rerank` tool drives. Its method set uses only stdlib
// types so a provider plugin satisfies it structurally without importing the
// kernel.
type Reranker interface {
	// Rerank returns, in descending relevance order, the original index of each
	// document and its relevance score (parallel slices). topN > 0 caps the
	// number returned.
	Rerank(ctx context.Context, query string, documents []string, topN int) ([]int, []float64, error)
	// HasRerank reports whether reranking is configured.
	HasRerank() bool
}

// rerankTool implements toolapi.Tool over a Reranker adapter.
type rerankTool struct {
	rr Reranker
}

// New returns the `rerank` agent tool over the given adapter.
func New(r Reranker) toolapi.Tool { return &rerankTool{rr: r} }

func (t *rerankTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:       "rerank",
		Capability: toolapi.ToolCapability{Name: string(edict.CapProviderCall)},
		Description: "Reorder candidate documents by relevance to a query using a dedicated reranking model — more " +
			"accurate than embedding similarity for picking the best few of many candidates. Returns the documents in " +
			"descending relevance order with scores. Optional top_n caps how many are returned.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type": "string", "description": "The information need to rank against"},
    "documents": {"type": "array", "items": {"type": "string"}, "description": "Candidate documents to reorder"},
    "top_n": {"type": "integer", "description": "Return only the top N (default: all)"}
  },
  "required": ["query", "documents"]
}`),
		Effect: toolapi.ToolEffect{Class: toolapi.EffectReadOnly},
	}
}

type rerankToolInput struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type rankedItem struct {
	Rank  int     `json:"rank"`
	Index int     `json:"index"`
	Score float64 `json:"score"`
	Text  string  `json:"text"`
}

func (t *rerankTool) Invoke(ctx context.Context, input json.RawMessage) (toolapi.Result, error) {
	if t.rr == nil || !t.rr.HasRerank() {
		return toolapi.Result{Output: "reranking is not configured (set AGEZT_RERANK_URL + AGEZT_RERANK_MODEL)", IsError: true}, nil
	}
	var in rerankToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return toolapi.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Query) == "" {
		return toolapi.Result{Output: "rerank needs a query", IsError: true}, nil
	}
	if len(in.Documents) == 0 {
		return toolapi.Result{Output: "rerank needs at least one document", IsError: true}, nil
	}
	idx, scores, err := t.rr.Rerank(ctx, in.Query, in.Documents, in.TopN)
	if err != nil {
		return toolapi.Result{Output: "rerank failed: " + err.Error(), IsError: true}, nil
	}
	ranked := make([]rankedItem, 0, len(idx))
	for i := range idx {
		if idx[i] < 0 || idx[i] >= len(in.Documents) {
			return toolapi.Result{Output: "rerank returned out-of-range index " + strconv.Itoa(idx[i]) + " for " + strconv.Itoa(len(in.Documents)) + " documents", IsError: true}, nil
		}
		ranked = append(ranked, rankedItem{Rank: i + 1, Index: idx[i], Score: scores[i], Text: in.Documents[idx[i]]})
	}
	out, err := json.Marshal(ranked)
	if err != nil {
		return toolapi.Result{Output: "rerank: encode result: " + err.Error(), IsError: true}, nil
	}
	return toolapi.Result{Output: string(out)}, nil
}
