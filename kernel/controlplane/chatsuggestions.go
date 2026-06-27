// SPDX-License-Identifier: MIT

package controlplane

// Chat Suggestions (M998): context-aware suggested next prompts shown after a
// chat turn completes. The suggestions are generated server-side based on the
// conversation context (recent tool calls, topics, and conversation state) so
// they are consistent across browser sessions and don't require LLM calls.

import (
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/memory"
)

// ChatSuggestion represents one clickable suggestion prompt.
type ChatSuggestion struct {
	// ID is a stable identifier for deduplication/tracking.
	ID string `json:"id"`
	// Label is the short display text shown on the chip.
	Label string `json:"label"`
	// Prompt is the text inserted into the chat input when clicked.
	Prompt string `json:"prompt"`
	// Category groups suggestions visually (e.g., "debug", "explore", "modify").
	Category string `json:"category"`
	// Icon is an optional icon name from the UI icon set.
	Icon string `json:"icon,omitempty"`
}

// handleChatSuggestions returns 3-5 context-aware suggested next prompts
// based on the recent conversation turns and tool activity. The session_id
// arg is optional (used for future per-session suggestion memory).
// Args: session_id (string, optional), turns ([{role,text}], optional),
//
//	tools ([string], optional — recently used tool names).
//
// Returns: { suggestions: [ChatSuggestion] }.
func (s *Server) handleChatSuggestions(conn net.Conn, req Request) {
	var sessionID string
	if v, ok := req.Args["session_id"].(string); ok {
		sessionID = v
	}

	// Collect context from args. The HTTP read-args proxy forwards each query
	// param as a single string, so the browser sends recently-used tool names
	// comma-joined (e.g. "write,bash"); accept a JSON array too for direct
	// control-plane callers.
	var recentTools []string
	switch v := req.Args["tools"].(type) {
	case string:
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				recentTools = append(recentTools, t)
			}
		}
	case []any:
		for _, t := range v {
			if s, ok := t.(string); ok && strings.TrimSpace(s) != "" {
				recentTools = append(recentTools, strings.TrimSpace(s))
			}
		}
	}

	// Memory-derived suggestions lead: turn the agent's active memory into
	// concrete starter/next-step prompts. Best-effort — a missing manager or a
	// read error just means we fall back to the tool-context catalog.
	var suggestions []ChatSuggestion
	if mgr := s.k.Memory(); mgr != nil {
		if recs, err := mgr.Active(); err == nil {
			suggestions = memorySuggestions(recs, maxMemorySuggestions)
		}
	}

	// Fill the remaining slots from the tool-context catalog, deduped by ID.
	suggestions = appendUnique(suggestions, buildSuggestions(sessionID, recentTools), maxSuggestions)

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"suggestions": suggestions,
		},
	})
}

// buildSuggestions returns the full suggestion catalog filtered by context.
// In the future this can be enhanced to do LLM-guided suggestion generation
// (TaskType "suggest") using the conversation transcript. For now it returns
// a static catalog of common actions, filtered by recently-used tools.
func buildSuggestions(sessionID string, recentTools []string) []ChatSuggestion {
	// Build a set of recent tool names for fast lookup.
	toolSet := make(map[string]bool)
	for _, t := range recentTools {
		toolSet[strings.ToLower(t)] = true
	}

	// The full catalog of possible suggestions.
	// In a future iteration, an LLM can generate dynamic suggestions based on
	// the conversation transcript (TaskType "suggest").
	allSuggestions := []ChatSuggestion{
		// Debug / inspect
		{ID: "debug-why", Label: "Why did this happen?", Prompt: "Explain why this result occurred in detail", Category: "debug", Icon: "search"},
		{ID: "debug-root-cause", Label: "Find the root cause", Prompt: "Trace back to the root cause of this issue", Category: "debug", Icon: "stethoscope"},
		{ID: "debug-alternatives", Label: "Show alternatives", Prompt: "What are the alternative approaches to solve this?", Category: "debug", Icon: "git-branch"},

		// Explore / learn
		{ID: "explore-deepen", Label: "Tell me more", Prompt: "Go deeper into this topic with more details and examples", Category: "explore", Icon: "book-open"},
		{ID: "explore-related", Label: "What else is related?", Prompt: "What related concepts or tools should I know about?", Category: "explore", Icon: "link"},
		{ID: "explore-tradeoffs", Label: "Trade-offs?", Prompt: "What are the trade-offs and pros/cons of this approach?", Category: "explore", Icon: "scale"},

		// Modify / act
		{ID: "modify-implement", Label: "Implement this", Prompt: "Write and run the code to implement this", Category: "modify", Icon: "play"},
		{ID: "modify-test", Label: "Test it", Prompt: "Write tests to verify this works correctly", Category: "modify", Icon: "check-circle"},
		{ID: "modify-fix", Label: "Fix the issue", Prompt: "Fix the issue described and verify the fix", Category: "modify", Icon: "wrench"},
		{ID: "modify-refactor", Label: "Refactor", Prompt: "Refactor this code to be cleaner and more maintainable", Category: "modify", Icon: "refresh-cw"},

		// Review / validate
		{ID: "review-security", Label: "Security review", Prompt: "Review this code for security vulnerabilities", Category: "review", Icon: "shield"},
		{ID: "review-performance", Label: "Performance check", Prompt: "Analyze this code for performance issues", Category: "review", Icon: "zap"},
		{ID: "review-best-practices", Label: "Best practices?", Prompt: "Does this follow best practices? What could be improved?", Category: "review", Icon: "star"},

		// Workflow
		{ID: "workflow-repeat", Label: "Do the same for...", Prompt: "Apply the same approach to the following similar case:", Category: "workflow", Icon: "copy"},
		{ID: "workflow-summarize", Label: "Summarize", Prompt: "Summarize what we discussed in a concise overview", Category: "workflow", Icon: "list"},
		{ID: "workflow-next-steps", Label: "Next steps", Prompt: "What should I do next to continue from here?", Category: "workflow", Icon: "arrow-right"},

		// Roster agent creation
		{ID: "agent-create-roster", Label: "Create roster agent", Prompt: "Create a new roster agent with the following configuration:", Category: "workflow", Icon: "bot"},
	}

	// If no context, return a default set.
	if len(recentTools) == 0 {
		return allSuggestions[:4]
	}

	// Otherwise, pick suggestions relevant to the recent tools.
	var relevant []ChatSuggestion

	// File-editing tools → suggest modify/review.
	if toolSet["write"] || toolSet["edit"] || toolSet["replace"] || toolSet["patch"] {
		relevant = append(relevant,
			ChatSuggestion{ID: "modify-test", Label: "Test it", Prompt: "Write tests to verify this code works correctly", Category: "modify", Icon: "check-circle"},
			ChatSuggestion{ID: "review-security", Label: "Security review", Prompt: "Review this code for security vulnerabilities", Category: "review", Icon: "shield"},
			ChatSuggestion{ID: "modify-refactor", Label: "Refactor", Prompt: "Refactor this code to be cleaner and more maintainable", Category: "modify", Icon: "refresh-cw"},
		)
	}

	// Shell tools → suggest debug/alternatives.
	if toolSet["bash"] || toolSet["exec"] || toolSet["shell"] {
		relevant = append(relevant,
			ChatSuggestion{ID: "debug-why", Label: "Why did this happen?", Prompt: "Explain why this command produced this output", Category: "debug", Icon: "search"},
			ChatSuggestion{ID: "debug-alternatives", Label: "Show alternatives", Prompt: "What are alternative ways to accomplish the same task?", Category: "debug", Icon: "git-branch"},
		)
	}

	// Web/search tools → suggest explore.
	if toolSet["web_search"] || toolSet["websearch"] || toolSet["fetch"] || toolSet["web_fetch"] {
		relevant = append(relevant,
			ChatSuggestion{ID: "explore-deepen", Label: "Tell me more", Prompt: "Go deeper into this topic with more details and examples", Category: "explore", Icon: "book-open"},
			ChatSuggestion{ID: "explore-tradeoffs", Label: "Trade-offs?", Prompt: "What are the trade-offs of this approach compared to alternatives?", Category: "explore", Icon: "scale"},
		)
	}

	// Git tools → suggest review.
	if toolSet["git"] || toolSet["git_status"] || toolSet["git_log"] {
		relevant = append(relevant,
			ChatSuggestion{ID: "review-best-practices", Label: "Best practices?", Prompt: "Review this git workflow for best practices", Category: "review", Icon: "star"},
			ChatSuggestion{ID: "workflow-next-steps", Label: "Next steps", Prompt: "What should I do next with this change?", Category: "workflow", Icon: "arrow-right"},
		)
	}

	// If we found relevant suggestions, deduplicate and return up to 4.
	if len(relevant) > 0 {
		seen := make(map[string]bool)
		var unique []ChatSuggestion
		for _, s := range relevant {
			if !seen[s.ID] {
				seen[s.ID] = true
				unique = append(unique, s)
			}
		}
		if len(unique) > 4 {
			unique = unique[:4]
		}
		return unique
	}

	// Fallback: generic suggestions.
	return []ChatSuggestion{
		{ID: "explore-deepen", Label: "Tell me more", Prompt: "Go deeper into this topic with more details and examples", Category: "explore", Icon: "book-open"},
		{ID: "modify-implement", Label: "Implement this", Prompt: "Write and run the code to implement this", Category: "modify", Icon: "play"},
		{ID: "workflow-next-steps", Label: "Next steps", Prompt: "What should I do next to continue from here?", Category: "workflow", Icon: "arrow-right"},
		{ID: "review-best-practices", Label: "Best practices?", Prompt: "Does this follow best practices? What could be improved?", Category: "review", Icon: "star"},
	}
}

const (
	// maxSuggestions caps the total chips returned per request.
	maxSuggestions = 5
	// maxMemorySuggestions caps how many of those come from memory, leaving room
	// for tool-context suggestions so the bar is a mix, not all-memory.
	maxMemorySuggestions = 3
	// memorySnippetLen bounds how much memory Content is inlined into a prompt.
	memorySnippetLen = 140
)

// memorySuggestions turns the agent's active memory records into concrete
// suggested prompts. It is pure (takes records, not a live kernel) so it can be
// unit-tested directly. High-signal records lead, deduped by subject; at most
// max are returned.
func memorySuggestions(recs []memory.Record, max int) []ChatSuggestion {
	if max <= 0 {
		return nil
	}
	// Keep only high-signal, subject-bearing records. OBSERVATION is noisy and
	// low-confidence, so it's excluded.
	var pick []memory.Record
	for _, r := range recs {
		if strings.TrimSpace(r.Subject) == "" {
			continue
		}
		switch r.Type {
		case memory.TypePreference, memory.TypeSummary, memory.TypeFact, memory.TypeRelation:
			pick = append(pick, r)
		}
	}
	// Strongest and most-recently-reinforced first.
	sort.SliceStable(pick, func(i, j int) bool {
		if pick[i].Confidence != pick[j].Confidence {
			return pick[i].Confidence > pick[j].Confidence
		}
		return pick[i].LastSeenMS > pick[j].LastSeenMS
	})

	seen := make(map[string]bool)
	var out []ChatSuggestion
	for _, r := range pick {
		key := strings.ToLower(strings.TrimSpace(r.Subject))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, memorySuggestion(r))
		if len(out) >= max {
			break
		}
	}
	return out
}

// memorySuggestion phrases one record as a clickable prompt, varying the wording
// by record type. The ID is subject-derived so it dedupes against itself across
// requests.
func memorySuggestion(r memory.Record) ChatSuggestion {
	subject := strings.TrimSpace(r.Subject)
	snippet := snip(r.Content, memorySnippetLen)
	id := "mem-" + strings.ToLower(strings.ReplaceAll(subject, " ", "-"))
	switch r.Type {
	case memory.TypePreference:
		prompt := "Keep my preference about " + subject + " in mind"
		if snippet != "" {
			prompt += " (" + snippet + ")"
		}
		prompt += " and apply it now."
		return ChatSuggestion{ID: id, Label: "Apply: " + subject, Prompt: prompt, Category: "memory", Icon: "brain"}
	case memory.TypeSummary:
		prompt := "Continue the work on " + subject + "."
		if snippet != "" {
			prompt += " So far: " + snippet
		}
		return ChatSuggestion{ID: id, Label: "Continue: " + subject, Prompt: prompt, Category: "memory", Icon: "brain"}
	default: // FACT, RELATION
		prompt := "Use what you know about " + subject + " to help me."
		if snippet != "" {
			prompt += " (Recall: " + snippet + ")"
		}
		return ChatSuggestion{ID: id, Label: "About " + subject, Prompt: prompt, Category: "memory", Icon: "brain"}
	}
}

// snip trims content to a single line of at most n runes, adding an ellipsis
// when truncated.
func snip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// appendUnique appends extras to base, skipping any whose ID is already present,
// and caps the result at limit.
func appendUnique(base, extras []ChatSuggestion, limit int) []ChatSuggestion {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s.ID] = true
	}
	out := base
	for _, s := range extras {
		if len(out) >= limit {
			break
		}
		if seen[s.ID] {
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
