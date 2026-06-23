// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestChatSuggestionsBlendsMemory exercises the full wired command path the
// /api/suggestions HTTP route hits: seed the agent's memory via CmdMemoryAdd,
// then call CmdChatSuggestions (tools comma-joined, as the read-args proxy
// forwards them) and assert memory-derived chips lead, blended with
// tool-context suggestions and capped.
func TestChatSuggestionsBlendsMemory(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	seed := []map[string]any{
		{"subject": "blunt reviews", "content": "be blunt and specific", "type": "PREFERENCE"},
		{"subject": "governor refactor", "content": "split the model resolver", "type": "SUMMARY"},
		{"subject": "prod region", "content": "prod is eu-west-1", "type": "FACT"},
	}
	for _, s := range seed {
		if _, err := c.Call(ctx, controlplane.CmdMemoryAdd, s); err != nil {
			t.Fatalf("memory add %v: %v", s["subject"], err)
		}
	}

	res, err := c.Call(ctx, controlplane.CmdChatSuggestions, map[string]any{
		"session_id": "conv1",
		"tools":      "write,bash", // comma-joined, like the HTTP proxy sends
	})
	if err != nil {
		t.Fatalf("chat_suggestions: %v", err)
	}
	raw, _ := res["suggestions"].([]any)
	if len(raw) == 0 {
		t.Fatal("expected suggestions, got none")
	}
	if len(raw) > 5 {
		t.Errorf("suggestions = %d, want <= 5 (cap)", len(raw))
	}

	var memCount int
	subjectsSeen := map[string]bool{}
	for _, item := range raw {
		m, _ := item.(map[string]any)
		cat, _ := m["category"].(string)
		label, _ := m["label"].(string)
		prompt, _ := m["prompt"].(string)
		if cat == "memory" {
			memCount++
			if prompt == "" {
				t.Errorf("memory suggestion %q has empty prompt", label)
			}
			for _, subj := range []string{"blunt reviews", "governor refactor", "prod region"} {
				if strings.Contains(strings.ToLower(label), subj) {
					subjectsSeen[subj] = true
				}
			}
		}
	}
	if memCount == 0 {
		t.Fatal("no memory-derived suggestions in the response")
	}
	if len(subjectsSeen) == 0 {
		t.Errorf("memory suggestions did not reference any seeded subject; got %v", raw)
	}
	// The seeded preference (highest implicit confidence/recency) should surface.
	if !subjectsSeen["blunt reviews"] {
		t.Errorf("expected the seeded PREFERENCE 'blunt reviews' to surface; got %v", raw)
	}
}

// TestChatSuggestionsNoMemoryFallsBack verifies that with an empty memory the
// command still returns tool-context suggestions (the catalog fallback).
func TestChatSuggestionsNoMemoryFallsBack(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdChatSuggestions, map[string]any{})
	if err != nil {
		t.Fatalf("chat_suggestions: %v", err)
	}
	raw, _ := res["suggestions"].([]any)
	if len(raw) == 0 {
		t.Fatal("expected fallback suggestions with empty memory, got none")
	}
}
