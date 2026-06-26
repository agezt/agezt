// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/agezt/agezt/kernel/agent"
)

const toolSearchName = "tool_search"

type toolSearchTool struct {
	defs []agent.ToolDef
}

func withToolSearch(tools map[string]agent.Tool) map[string]agent.Tool {
	if len(tools) == 0 {
		return tools
	}
	out := make(map[string]agent.Tool, len(tools)+1)
	for name, t := range tools {
		out[name] = t
	}
	if _, exists := out[toolSearchName]; exists {
		return out
	}
	defs := make([]agent.ToolDef, 0, len(tools))
	for name, t := range tools {
		if name == toolSearchName {
			continue
		}
		defs = append(defs, t.Definition())
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	out[toolSearchName] = toolSearchTool{defs: defs}
	return out
}

func (t toolSearchTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name:        toolSearchName,
		Description: "Search the deferred tool catalog by capability before asking for a specific tool schema. Use when the visible tools do not include the capability you need.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {"type":"string", "description":"Capability or task to search for, e.g. \"read files\", \"send notification\", \"calendar\"."},
    "limit": {"type":"integer", "description":"Maximum matches to return. Default 8, maximum 20."}
  }
}`),
		Effect: agent.ToolEffect{
			Class:             agent.EffectReadOnly,
			PredictedEffects:  []string{"Read the in-run catalog of available tool names and descriptions."},
			AffectedResources: []string{"tool schema context"},
			Confidence:        0.95,
		},
	}
}

func (t toolSearchTool) Invoke(_ context.Context, raw json.RawMessage) (agent.Result, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(raw, &in)
	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	q := toolSearchTokens(in.Query)
	type row struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Score       int    `json:"score,omitempty"`
	}
	rows := make([]row, 0, len(t.defs))
	for _, def := range t.defs {
		score := toolSearchScore(q, def)
		if len(q) > 0 && score == 0 {
			continue
		}
		rows = append(rows, row{Name: def.Name, Description: firstSentence(def.Description), Score: score})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return rows[i].Name < rows[j].Name
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	body, err := json.MarshalIndent(map[string]any{
		"query": strings.TrimSpace(in.Query),
		"count": len(rows),
		"tools": rows,
	}, "", "  ")
	if err != nil {
		return agent.Result{Output: "tool_search: marshal: " + err.Error(), IsError: true}, nil
	}
	return agent.Result{Output: string(body)}, nil
}

func toolSearchScore(query map[string]bool, def agent.ToolDef) int {
	if len(query) == 0 {
		return 0
	}
	hay := strings.ToLower(def.Name + " " + def.Description + " " + string(def.InputSchema))
	score := 0
	for tok := range query {
		if tok == strings.ToLower(def.Name) {
			score += 10
		}
		if strings.Contains(strings.ToLower(def.Name), tok) {
			score += 5
		}
		if strings.Contains(hay, tok) {
			score += 2
		}
	}
	return score
}

func toolSearchTokens(s string) map[string]bool {
	out := map[string]bool{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := b.String()
		b.Reset()
		if len(tok) < 3 {
			return
		}
		out[tok] = true
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}
