// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

// ToolSelectionRequest is the context a discovery layer receives before each
// provider call. It may return any subset of Tools; the loop will normalize that
// subset back to the original definitions by name so selectors cannot mutate
// schemas or invent tools.
type ToolSelectionRequest struct {
	Intent   string
	Iter     int
	Messages []Message
	Tools    []ToolDef
}

// ToolSelector chooses which registered tools should be offered on a provider
// call. Nil means "offer every available tool" (the historical behaviour).
type ToolSelector func(ctx context.Context, req ToolSelectionRequest) ([]ToolDef, error)

// LexicalToolSelector returns a deterministic capability/semantic selector that
// ranks tools by overlap between the task text and each tool's name,
// description, and schema text. It is intentionally conservative: if no tool has
// a positive match, it returns all tools rather than hiding a needed capability.
// max <= 0 disables selection.
func LexicalToolSelector(max int) ToolSelector {
	return lexicalToolSelector(max, nil, true)
}

// DeferredLexicalToolSelector ranks tools like LexicalToolSelector, but keeps
// pinned catalog/search tools visible and does not fall back to exposing every
// schema on a no-match turn. This is the small-schema mode: the model can call a
// pinned discovery tool to promote the right capability into later context.
func DeferredLexicalToolSelector(max int, pinned []string) ToolSelector {
	return lexicalToolSelector(max, pinned, false)
}

func lexicalToolSelector(max int, pinned []string, fallbackAll bool) ToolSelector {
	if max <= 0 {
		return nil
	}
	pinnedSet := map[string]bool{}
	for _, name := range pinned {
		if strings.TrimSpace(name) != "" {
			pinnedSet[name] = true
		}
	}
	return func(_ context.Context, req ToolSelectionRequest) ([]ToolDef, error) {
		if len(req.Tools) <= max {
			return append([]ToolDef(nil), req.Tools...), nil
		}
		query := req.Intent
		for _, m := range req.Messages {
			if m.Role == RoleUser {
				query += " " + m.Content
			}
		}
		q := tokenSet(query)
		if len(q) == 0 {
			if fallbackAll {
				return append([]ToolDef(nil), req.Tools...), nil
			}
			return pinnedTools(req.Tools, pinnedSet), nil
		}

		type scored struct {
			def   ToolDef
			score int
			index int
		}
		rows := make([]scored, 0, len(req.Tools))
		for i, t := range req.Tools {
			if pinnedSet[t.Name] {
				continue
			}
			rows = append(rows, scored{def: t, score: toolScore(q, t), index: i})
		}
		pinnedOut := pinnedTools(req.Tools, pinnedSet)
		if len(rows) == 0 {
			return pinnedOut, nil
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].score != rows[j].score {
				return rows[i].score > rows[j].score
			}
			if rows[i].def.Name != rows[j].def.Name {
				return rows[i].def.Name < rows[j].def.Name
			}
			return rows[i].index < rows[j].index
		})
		if rows[0].score <= 0 {
			if fallbackAll {
				return append([]ToolDef(nil), req.Tools...), nil
			}
			return pinnedOut, nil
		}
		limit := max
		if limit > len(rows) {
			limit = len(rows)
		}
		out := make([]ToolDef, 0, len(pinnedOut)+limit)
		out = append(out, pinnedOut...)
		selected := 0
		for _, row := range rows {
			if row.score <= 0 || selected >= limit {
				break
			}
			out = append(out, row.def)
			selected++
		}
		return out, nil
	}
}

func pinnedTools(tools []ToolDef, pinned map[string]bool) []ToolDef {
	if len(pinned) == 0 {
		return nil
	}
	out := make([]ToolDef, 0, len(pinned))
	for _, t := range tools {
		if pinned[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func normalizeSelectedTools(available, selected []ToolDef) []ToolDef {
	byName := make(map[string]ToolDef, len(available))
	for _, t := range available {
		byName[t.Name] = t
	}
	out := make([]ToolDef, 0, len(selected))
	seen := map[string]bool{}
	for _, t := range selected {
		if seen[t.Name] {
			continue
		}
		if original, ok := byName[t.Name]; ok {
			out = append(out, original)
			seen[t.Name] = true
		}
	}
	return out
}

func toolScore(query map[string]bool, t ToolDef) int {
	name := strings.ToLower(t.Name)
	desc := strings.ToLower(t.Description)
	score := 0
	for q := range query {
		if q == name {
			score += 12
		}
		if strings.Contains(name, q) {
			score += 6
		}
		if strings.Contains(desc, q) {
			score += 2
		}
	}
	for tok := range tokenSet(t.Name + " " + t.Description + " " + string(t.InputSchema)) {
		if query[tok] {
			score += 3
		}
	}
	return score
}

func tokenSet(s string) map[string]bool {
	tokens := map[string]bool{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := b.String()
		b.Reset()
		if len(tok) < 3 || toolSelectStopwords[tok] {
			return
		}
		tokens[tok] = true
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

var toolSelectStopwords = map[string]bool{
	"and": true, "are": true, "for": true, "from": true, "has": true,
	"input": true, "object": true, "one": true, "properties": true,
	"required": true, "schema": true, "string": true, "that": true,
	"the": true, "this": true, "tool": true, "type": true, "with": true,
}
