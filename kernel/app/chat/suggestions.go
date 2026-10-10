// SPDX-License-Identifier: MIT

// Package chat owns the chat surface's context-aware next-prompt suggestions
// (M998), chips derived from the agent's active memory and the recently used
// tools with no LLM call, and the history summarizer (M925), one bounded
// provider call that folds older turns into a briefing.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Suggestion represents one clickable suggestion prompt.
type Suggestion struct {
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

// Ports reads the agent's active memory. A nil port, like a read error, means
// no memory-derived suggestions.
type Ports struct {
	ActiveMemory func() ([]memory.Record, error)
}

type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

// SuggestionsRequest carries the optional session id and the recently used
// tool names: comma-joined, as the Web UI's read proxy forwards them, or a
// JSON array for direct callers.
type SuggestionsRequest struct {
	SessionID json.RawMessage `json:"session_id,omitempty"`
	Tools     json.RawMessage `json:"tools,omitempty"`
}

type SuggestionsOutput struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestions returns up to five chips: memory-derived ones lead, at most
// three, and the tool-context catalog fills the rest, deduped by ID. A present
// session id must be a string; tool names that are neither a string nor an
// array are ignored, as are non-string array entries.
func (s *Service) Suggestions(_ context.Context, in SuggestionsRequest) (SuggestionsOutput, error) {
	var sessionID string
	if len(in.SessionID) != 0 {
		var v any
		_ = json.Unmarshal(in.SessionID, &v)
		str, ok := v.(string)
		if !ok {
			return SuggestionsOutput{}, errors.New("args.session_id must be a string")
		}
		sessionID = str
	}
	var recentTools []string
	if len(in.Tools) != 0 {
		var v any
		_ = json.Unmarshal(in.Tools, &v)
		switch v := v.(type) {
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
	}
	var suggestions []Suggestion
	if s.ports.ActiveMemory != nil {
		if recs, err := s.ports.ActiveMemory(); err == nil {
			suggestions = memorySuggestions(recs, maxMemorySuggestions)
		}
	}
	suggestions = appendUnique(suggestions, buildSuggestions(sessionID, recentTools), maxSuggestions)
	return SuggestionsOutput{Suggestions: suggestions}, nil
}

// Operations declares the read-only, operator-only suggestions on their Web UI
// route.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("chat provider required")
	}
	out, err := schema.FromType(reflect.TypeFor[SuggestionsOutput](), false)
	if err != nil {
		return nil, err
	}
	op, err := app.NewOperation(opapi.Spec{Name: "chat_suggestions", ReadOnly: true, OutputSchema: out, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"session_id":{},"tools":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/suggestions"}}, func(ctx context.Context, in SuggestionsRequest) (SuggestionsOutput, error) {
		return provider(ctx).Suggestions(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}

// buildSuggestions returns the full suggestion catalog filtered by context.
// In the future this can be enhanced to do LLM-guided suggestion generation
// (TaskType "suggest") using the conversation transcript. For now it returns
// a static catalog of common actions, filtered by recently-used tools.
func buildSuggestions(sessionID string, recentTools []string) []Suggestion {
	// Build a set of recent tool names for fast lookup.
	toolSet := make(map[string]bool)
	for _, t := range recentTools {
		toolSet[strings.ToLower(t)] = true
	}

	// The full catalog of possible suggestions.
	// In a future iteration, an LLM can generate dynamic suggestions based on
	// the conversation transcript (TaskType "suggest").
	allSuggestions := []Suggestion{
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
	var relevant []Suggestion

	// File-editing tools → suggest modify/review.
	if toolSet["write"] || toolSet["edit"] || toolSet["replace"] || toolSet["patch"] {
		relevant = append(relevant,
			Suggestion{ID: "modify-test", Label: "Test it", Prompt: "Write tests to verify this code works correctly", Category: "modify", Icon: "check-circle"},
			Suggestion{ID: "review-security", Label: "Security review", Prompt: "Review this code for security vulnerabilities", Category: "review", Icon: "shield"},
			Suggestion{ID: "modify-refactor", Label: "Refactor", Prompt: "Refactor this code to be cleaner and more maintainable", Category: "modify", Icon: "refresh-cw"},
		)
	}

	// Shell tools → suggest debug/alternatives.
	if toolSet["bash"] || toolSet["exec"] || toolSet["shell"] {
		relevant = append(relevant,
			Suggestion{ID: "debug-why", Label: "Why did this happen?", Prompt: "Explain why this command produced this output", Category: "debug", Icon: "search"},
			Suggestion{ID: "debug-alternatives", Label: "Show alternatives", Prompt: "What are alternative ways to accomplish the same task?", Category: "debug", Icon: "git-branch"},
		)
	}

	// Web/search tools → suggest explore.
	if toolSet["web_search"] || toolSet["websearch"] || toolSet["fetch"] || toolSet["web_fetch"] {
		relevant = append(relevant,
			Suggestion{ID: "explore-deepen", Label: "Tell me more", Prompt: "Go deeper into this topic with more details and examples", Category: "explore", Icon: "book-open"},
			Suggestion{ID: "explore-tradeoffs", Label: "Trade-offs?", Prompt: "What are the trade-offs of this approach compared to alternatives?", Category: "explore", Icon: "scale"},
		)
	}

	// Git tools → suggest review.
	if toolSet["git"] || toolSet["git_status"] || toolSet["git_log"] {
		relevant = append(relevant,
			Suggestion{ID: "review-best-practices", Label: "Best practices?", Prompt: "Review this git workflow for best practices", Category: "review", Icon: "star"},
			Suggestion{ID: "workflow-next-steps", Label: "Next steps", Prompt: "What should I do next with this change?", Category: "workflow", Icon: "arrow-right"},
		)
	}

	// If we found relevant suggestions, deduplicate and return up to 4.
	if len(relevant) > 0 {
		seen := make(map[string]bool)
		var unique []Suggestion
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
	return []Suggestion{
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
func memorySuggestions(recs []memory.Record, max int) []Suggestion {
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
	var out []Suggestion
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
func memorySuggestion(r memory.Record) Suggestion {
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
		return Suggestion{ID: id, Label: "Apply: " + subject, Prompt: prompt, Category: "memory", Icon: "brain"}
	case memory.TypeSummary:
		prompt := "Continue the work on " + subject + "."
		if snippet != "" {
			prompt += " So far: " + snippet
		}
		return Suggestion{ID: id, Label: "Continue: " + subject, Prompt: prompt, Category: "memory", Icon: "brain"}
	default: // FACT, RELATION
		prompt := "Use what you know about " + subject + " to help me."
		if snippet != "" {
			prompt += " (Recall: " + snippet + ")"
		}
		return Suggestion{ID: id, Label: "About " + subject, Prompt: prompt, Category: "memory", Icon: "brain"}
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
func appendUnique(base, extras []Suggestion, limit int) []Suggestion {
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
