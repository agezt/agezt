// SPDX-License-Identifier: MIT

// Package convoapi is the contract layer's home for the conversation
// collapse: the one conversational turn, and the rule that folds a
// multi-turn conversation into a single Agezt intent.
//
// The rule lives here because the surfaces that need it are ADAPTERS: the
// OpenAI-compatible API and the Web UI Chat view both render prior turns
// the same way, and an adapter may reach the contract layer and the
// platform layer and nothing else. The rule used to live in kernel/convo —
// a layer-3 package — so both adapters had to name it, which is the
// adapter-bypass the ratchet was counting (W3.65). The row and the rule
// are pure, so they belong in the layer that every layer may depend on.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No store, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package convoapi

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
