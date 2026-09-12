// SPDX-License-Identifier: MIT

// WebUI security helpers: toStr + stringList + historyTurns.
// Code extracted from webui_security.go during the Day-75 god-file split. Public API unchanged.
package webui


import (
	"github.com/agezt/agezt/kernel/convo"
	"strings"
)


func toStr(v any) string {
	s, _ := v.(string)
	return s
}

// stringList coerces a decoded JSON array of strings (body value) to []string.
func stringList(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// maxHistoryTurns bounds how many prior turns the Chat view can fold into one
// run's intent, so a long thread can't grow the intent without limit. The most
// recent turns are kept (the tail carries the live context).
const maxHistoryTurns = 40

// historyTurns parses the optional `history` body field — a JSON array of
// {role, text} objects (as decoded into []any of map[string]any) — into convo
// turns, skipping malformed/blank entries and keeping only the most recent
// maxHistoryTurns. Returns nil when there is no usable history (the single-shot
// path, unchanged).
func historyTurns(raw any) []convo.Turn {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	turns := make([]convo.Turn, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		text, _ := m["text"].(string)
		if strings.TrimSpace(role) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		turns = append(turns, convo.Turn{Role: role, Text: text})
	}
	if len(turns) > maxHistoryTurns {
		turns = turns[len(turns)-maxHistoryTurns:]
	}
	return turns
}

// secure applies the defensive response headers (CSP et al.) to a handler but
// does NOT require a token. Used for public, secret-free static subresources