// SPDX-License-Identifier: MIT

// Package convo collapses a multi-turn conversation into a single Agezt intent —
// the deliberate, lossy-by-design mapping that lets the single-intent governed
// loop carry conversational context. It is the one place that mapping lives, so
// every multi-turn surface (the OpenAI-compatible API, the Web UI Chat view)
// renders prior turns identically: a single user turn passes through verbatim;
// a real conversation becomes a labelled transcript ("User: …" / "Assistant: …")
// with any system/developer guidance hoisted to the front (the kernel still
// applies its own system prompt around this).
package convo

import "strings"

// Turn is one conversational message. Role is case-insensitive; "system" and
// "developer" are leading guidance, "user"/"assistant" are the dialogue, any
// other role is included verbatim in the transcript body.
type Turn struct {
	Role string
	Text string
}

// TranscriptIntent renders turns into one intent string. Empty-text turns are
// skipped. A lone user turn (no other dialogue) is returned verbatim so the
// common single-shot case is unchanged; otherwise the dialogue is a labelled
// transcript. System/developer turns, if any, are joined and prefixed.
func TranscriptIntent(turns []Turn) string {
	var systems, dialogue []string
	soleUser := ""
	userTurns := 0
	for _, t := range turns {
		text := strings.TrimSpace(t.Text)
		if text == "" {
			continue
		}
		switch strings.ToLower(t.Role) {
		case "system", "developer":
			systems = append(systems, text)
		case "user":
			userTurns++
			soleUser = text
			dialogue = append(dialogue, "User: "+text)
		case "assistant":
			dialogue = append(dialogue, "Assistant: "+text)
		default:
			dialogue = append(dialogue, text)
		}
	}
	var b strings.Builder
	if len(systems) > 0 {
		b.WriteString(strings.Join(systems, "\n"))
		b.WriteString("\n\n")
	}
	// Single user turn → clean intent (no transcript labels), preserving the
	// single-shot behaviour exactly.
	if userTurns == 1 && len(dialogue) == 1 {
		b.WriteString(soleUser)
		return strings.TrimSpace(b.String())
	}
	b.WriteString(strings.Join(dialogue, "\n"))
	return strings.TrimSpace(b.String())
}
