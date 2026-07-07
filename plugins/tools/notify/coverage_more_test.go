// SPDX-License-Identifier: MIT

package notify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestNotifyCoverageKinds(t *testing.T) {
	// kinds is exported (lowercase) within the package; it returns the
	// map's keys sorted.
	maps := map[string]map[string][]string{
		"empty":              {"alpha": {"a1"}, "beta": {"b1"}},
		"single":             {"telegram": {"T1"}},
		"deterministic-sort": {"z": {"z1"}, "a": {"a1"}, "m": {"m1"}, "b": {"b1"}},
	}
	for name, in := range maps {
		got := kinds(in)
		// Must be sorted.
		if !sortedStrings(got) {
			t.Errorf("%s: %v not sorted", name, got)
		}
	}
	// Snapshot on unbound tool: empty send + empty targets.
	tl := New()
	send, targets := tl.snapshot()
	if send != nil {
		t.Fatal("snapshot should have nil send on unbound tool")
	}
	if len(targets) != 0 {
		t.Fatal("snapshot should have empty targets on unbound tool")
	}
}

func TestNotifyCoverageDefinition(t *testing.T) {
	// Unconfigured tool: description should mention "(none configured yet)".
	tool := New()
	def := tool.Definition()
	if def.Name != "notify" {
		t.Fatalf("Name = %q", def.Name)
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}
	if !strings.Contains(def.Description, "(none configured yet)") {
		t.Fatalf("description should list empty state, got %q", def.Description)
	}
	if !strings.Contains(def.Effect.AffectedResources[0], "(none configured yet)") {
		t.Fatalf("resources should list empty state, got %q", def.Effect.AffectedResources[0])
	}
	// Configured tool: kinds appear in description and resources.
	tool.Bind(func(context.Context, string, string, string) error { return nil }, map[string][]string{
		"telegram": {"T1"},
		"slack":    {"S1"},
	})
	def = tool.Definition()
	if !strings.Contains(def.Description, "slack, telegram") {
		t.Fatalf("description should list sorted kinds, got %q", def.Description)
	}
}

func TestNotifyCoverageInvokeBranches(t *testing.T) {
	// Parse error: soft.
	tool := New()
	tool.Bind(func(context.Context, string, string, string) error { return nil }, map[string][]string{"slack": {"S1"}})
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("parse error = %+v", res)
	}

	// Unknown channel: soft error listing available kinds.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi","channel":"twitter"}`))
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Fatalf("unknown channel = %+v", res)
	}
	if !strings.Contains(res.Output, "slack") {
		t.Fatalf("unknown channel output should list available kinds, got %s", res.Output)
	}

	// Channel filter: case-insensitive matching.
	var lastKind string
	tool.Bind(func(_ context.Context, kind, id, text string) error {
		lastKind = kind
		return nil
	}, map[string][]string{"telegram": {"T1"}, "slack": {"S1"}})
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi","channel":"TELEGRAM"}`))
	if res.IsError {
		t.Fatalf("case-insensitive channel should match, got %+v", res)
	}
	if lastKind != "telegram" {
		t.Fatalf("lastKind = %q, want telegram", lastKind)
	}
}

func TestNotifyCoverageSeverity(t *testing.T) {
	// Severity is parsed but the current sender signature is (kind, id, text);
	// the tool does NOT currently splice severity into the text. Document that
	// the field is accepted (no parse error) and verify the basic send path.
	var sentText string
	tool := New()
	tool.Bind(func(_ context.Context, _, _, text string) error {
		sentText = text
		return nil
	}, map[string][]string{"slack": {"S1"}})
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"text":"hi","severity":"critical"}`))
	if res.IsError {
		t.Fatalf("severity invoke = %+v", res)
	}
	if sentText != "hi" {
		t.Fatalf("expected text %q, got %q", "hi", sentText)
	}
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}
