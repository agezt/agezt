// SPDX-License-Identifier: MIT

package controlplane

// Chat Suggestions (M998): context-aware suggested next prompts shown after a
// chat turn completes. The suggestions are generated server-side based on the
// conversation context (recent tool calls, topics, and conversation state) so
// they are consistent across browser sessions and don't require LLM calls.

import (
	"net"
	"strings"
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
//       tools ([string], optional — recently used tool names).
// Returns: { suggestions: [ChatSuggestion] }.
func (s *Server) handleChatSuggestions(conn net.Conn, req Request) {
	var sessionID string
	if v, ok := req.Args["session_id"].(string); ok {
		sessionID = v
	}

	// Collect context from args.
	var recentTools []string
	if v, ok := req.Args["tools"].([]any); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				recentTools = append(recentTools, s)
			}
		}
	}

	// Build the suggestion list based on context.
	suggestions := buildSuggestions(sessionID, recentTools)

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
