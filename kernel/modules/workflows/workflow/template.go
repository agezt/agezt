// SPDX-License-Identifier: MIT

package workflow

// {{path}} template interpolation — how data flows between nodes. A path
// walks the run context: {{trigger.payload}}, {{<node_id>.output}}, and any
// deeper JSON the upstream node produced ({{fetch.output.items.0.title}}).
// Deliberately tiny: dotted lookups only, no expressions, no pipes — a
// transform node (or, later, a code node) is where real reshaping happens.
// Unknown paths resolve to "" rather than failing the run: a half-filled
// template is visible in the output and easy to debug from the journal.

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Interpolate replaces every {{path}} in s with the value at that path in
// data. Strings insert verbatim; everything else inserts as compact JSON.
func Interpolate(s string, data map[string]any) string {
	var b strings.Builder
	for {
		start := strings.Index(s, "{{")
		if start < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := strings.Index(s[start:], "}}")
		if end < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:start])
		path := strings.TrimSpace(s[start+2 : start+end])
		b.WriteString(renderValue(Lookup(data, path)))
		s = s[start+end+2:]
	}
}

// Lookup resolves a dotted path against nested maps/slices (JSON shapes).
// Array steps are decimal indexes. A miss anywhere returns nil.
func Lookup(data map[string]any, path string) any {
	if path == "" {
		return nil
	}
	var cur any = data
	for step := range strings.SplitSeq(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[step]
			if !ok {
				return nil
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(step)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			cur = v[idx]
		default:
			return nil
		}
	}
	return cur
}

func renderValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
