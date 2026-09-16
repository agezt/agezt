// SPDX-License-Identifier: MIT
//
// kernel/controlplane tool-log preview helper (previewString + toolOutputPreviewRunes).
// Extracted from tool_log.go during Day 211 god-file refactor (#78).
// Public API unchanged.
package controlplane

import (
	"strings"
)

// toolOutputPreviewRunes bounds the one-line output/input excerpt folded into a
// tool-log row — long enough to read an error message or a short result, short
// enough to keep the response compact. Mirrors answerPreviewRunes' role for runs.
const toolOutputPreviewRunes = 100

func previewString(s string) string {
	one := strings.Join(strings.Fields(s), " ")
	if one == "" {
		return ""
	}
	r := []rune(one)
	if len(r) > toolOutputPreviewRunes {
		return string(r[:toolOutputPreviewRunes]) + "…"
	}
	return one
}
