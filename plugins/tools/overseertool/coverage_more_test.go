// SPDX-License-Identifier: MIT

package overseertool

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// --- parseProfile edge cases ---

func TestParseProfile_NullAndEmptyProfileRaw(t *testing.T) {
	src := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

	// Null profile raw → falls back to flat fields.
	_, err := parseProfile(src(nil), src(map[string]any{"slug": "a"}))
	if err != nil {
		t.Fatalf("null profile with flat fields: %v", err)
	}

	// Empty string profile raw is truthy but not a valid object → error.
	_, err = parseProfile(src(""), src(map[string]any{"slug": "b"}))
	if err == nil {
		t.Fatal("empty string profile raw should error (string is not an object)")
	}
}

func TestParseProfile_InvalidJSONInsideProfile(t *testing.T) {
	src := json.RawMessage(`{invalid}`)
	_, err := parseProfile(src, src)
	if err == nil {
		t.Fatal("parseProfile with invalid JSON should error")
	}
}

func TestParseProfile_NoProfileAndNoFlatFields(t *testing.T) {
	src := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	_, err := parseProfile(src(nil), src(map[string]any{}))
	if err == nil {
		t.Fatal("parseProfile with no profile and no flat fields should error")
	}
}

// --- hasProfileObject ---

func TestHasProfileObject_Variants(t *testing.T) {
	cases := []struct {
		raw json.RawMessage
		ok  bool
	}{
		{json.RawMessage(`{}`), false},
		{json.RawMessage(`null`), false},
		{json.RawMessage(``), false},
		{json.RawMessage(`{"slug":"x"}`), true},
		{json.RawMessage(`0`), true}, // truthy but not an object — should pass
	}
	for _, tc := range cases {
		got := hasProfileObject(tc.raw)
		if got != tc.ok {
			t.Errorf("hasProfileObject(%q) = %v, want %v", string(tc.raw), got, tc.ok)
		}
	}
}

// --- cleanSlugs ---

func TestCleanSlugs_TrimAndDeduplicates(t *testing.T) {
	got := cleanSlugs([]string{"  a1  ", "", "  ", "a2", " a2 ", "a3"})
	// cleanSlugs trims and removes empties but does NOT deduplicate.
	if len(got) != 4 || got[0] != "a1" || got[3] != "a3" {
		t.Fatalf("cleanSlugs = %#v, want roughly [a1 a2 a2 a3]", got)
	}
}

func TestCleanSlugs_NilAndEmpty(t *testing.T) {
	if cleanSlugs(nil) != nil {
		t.Error("cleanSlugs(nil) should be nil")
	}
	if cleanSlugs([]string{}) != nil {
		t.Error("cleanSlugs(empty) should be nil")
	}
	if len(cleanSlugs([]string{"", "  "})) != 0 {
		t.Error("cleanSlugs(all empty) should be empty")
	}
}

// --- errResult ---

func TestErrResult_Format(t *testing.T) {
	r := errResult("something went wrong")
	if !r.IsError {
		t.Error("errResult should mark IsError")
	}
	if r.Output != "overseer: something went wrong" {
		t.Errorf("errResult output = %q", r.Output)
	}
}

// --- findMisconfigured / findDegraded / findRoutingPressure ---

func TestFindMisconfigured_FoundAndNotFound(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{
			{Slug: "alpha", Issues: []string{"bad override"}},
			{Slug: "beta", Issues: []string{"missing model"}},
		},
	}
	if findMisconfigured(rep, "alpha") == nil {
		t.Fatal("findMisconfigured(alpha) should find it")
	}
	if findMisconfigured(rep, "gamma") != nil {
		t.Fatal("findMisconfigured(gamma) should be nil")
	}
	if findMisconfigured(kernelruntime.ReaperReport{}, "alpha") != nil {
		t.Fatal("findMisconfigured(empty) should be nil")
	}
}

func TestFindDegraded_FoundAndNotFound(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		DegradedAgents: []kernelruntime.DegradedAgent{
			{Slug: "alpha", Failures: 5, Window: 10, LastReason: "timeout"},
		},
	}
	if findDegraded(rep, "alpha") == nil {
		t.Fatal("findDegraded(alpha) should find it")
	}
	if findDegraded(rep, "beta") != nil {
		t.Fatal("findDegraded(beta) should be nil")
	}
}

func TestFindRoutingPressure_FoundAndNotFound(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "alpha", Count: 3, Threshold: 3, TaskType: "code", LastFailedModel: "gpt-5"},
		},
	}
	if findRoutingPressure(rep, "alpha") == nil {
		t.Fatal("findRoutingPressure(alpha) should find it")
	}
	if findRoutingPressure(rep, "beta") != nil {
		t.Fatal("findRoutingPressure(beta) should be nil")
	}
	if findRoutingPressure(kernelruntime.ReaperReport{}, "alpha") != nil {
		t.Fatal("findRoutingPressure(empty) should be nil")
	}
}

// --- repairTaskType ---

func TestRepairTaskType_UsesProfileTaskType(t *testing.T) {
	tt := repairTaskType(roster.Profile{Slug: "a", TaskType: "research"}, kernelruntime.ReaperReport{})
	if tt != "research" {
		t.Fatalf("repairTaskType = %q, want research", tt)
	}
}

func TestRepairTaskType_FallsBackToRoutingPressure(t *testing.T) {
	tt := repairTaskType(roster.Profile{Slug: "b"}, kernelruntime.ReaperReport{
		RoutingPressure: []kernelruntime.RoutingPressureAgent{
			{Slug: "b", TaskType: "code"},
		},
	})
	if tt != "code" {
		t.Fatalf("repairTaskType (fallback) = %q, want code", tt)
	}
}

func TestRepairTaskType_EmptyWhenNoSignal(t *testing.T) {
	tt := repairTaskType(roster.Profile{Slug: "c"}, kernelruntime.ReaperReport{})
	if tt != "" {
		t.Fatalf("repairTaskType with no signal = %q, want empty", tt)
	}
}

// --- managedSubAgentRepairHint ---

func TestManagedSubAgentRepairHint_WithParent(t *testing.T) {
	hint := managedSubAgentRepairHint(roster.Profile{Slug: "sub", ParentAgent: "parent"})
	if hint == "" {
		t.Fatal("hint should not be empty")
	}
}

func TestManagedSubAgentRepairHint_WithOwnerOnly(t *testing.T) {
	hint := managedSubAgentRepairHint(roster.Profile{Slug: "sub", OwnerAgent: "owner"})
	if hint == "" {
		t.Fatal("hint should not be empty")
	}
}

func TestManagedSubAgentRepairHint_NoManager(t *testing.T) {
	hint := managedSubAgentRepairHint(roster.Profile{Slug: "sub"})
	if hint == "" {
		t.Fatal("hint should not be empty even without manager")
	}
}

// --- sanitizeTaskModelChain ---

func TestSanitizeTaskModelChain_RemovesEmpty(t *testing.T) {
	got := sanitizeTaskModelChain([]string{"model-1", "", "  ", "model-2"})
	if len(got) != 2 || got[0] != "model-1" || got[1] != "model-2" {
		t.Fatalf("sanitizeTaskModelChain = %#v", got)
	}
}

func TestSanitizeTaskModelChain_AllEmpty(t *testing.T) {
	got := sanitizeTaskModelChain([]string{"", "  "})
	if len(got) != 0 {
		t.Fatalf("sanitizeTaskModelChain(all empty) = %#v, want empty", got)
	}
}

// --- encodeTaskModelChains ---

func TestEncodeTaskModelChains_SortsByTaskType(t *testing.T) {
	got := encodeTaskModelChains(map[string][]string{
		"b": {"model-b"},
		"a": {"model-a"},
	})
	want := "a=model-a;b=model-b"
	if got != want {
		t.Fatalf("encodeTaskModelChains = %q, want %q", got, want)
	}
}

func TestEncodeTaskModelChains_Empty(t *testing.T) {
	if encodeTaskModelChains(nil) != "" {
		t.Error("encodeTaskModelChains(nil) should be empty")
	}
	if encodeTaskModelChains(map[string][]string{}) != "" {
		t.Error("encodeTaskModelChains(empty) should be empty")
	}
}

func TestEncodeTaskModelChains_SkipsEmptyValues(t *testing.T) {
	got := encodeTaskModelChains(map[string][]string{
		"":    {"model-a"},
		"b":   {},
		"c":   {"model-c"},
		"   ": {"model-d"},
	})
	if got != "c=model-c" {
		t.Fatalf("encodeTaskModelChains = %q, want %q", got, "c=model-c")
	}
}

// --- applyProfilePatchField ---

func TestApplyProfilePatchField_OnlyWhenProvided(t *testing.T) {
	var dst string
	applyProfilePatchField(map[string]bool{"key": true}, "key", &dst, "val")
	if dst != "val" {
		t.Fatalf("dst = %q, want val", dst)
	}
	applyProfilePatchField(map[string]bool{}, "key", &dst, "other")
	if dst != "val" {
		t.Fatalf("dst should NOT change when key not provided, got %q", dst)
	}
}

// --- clip edge cases ---

func TestClip_EdgeCases(t *testing.T) {
	if clip("x", 0) != "" {
		t.Errorf("clip('x', 0) = %q, want '' (empty from s[:0])", clip("x", 0))
	}
	if clip("xy", 1) != "x" {
		t.Errorf("clip('xy', 1) = %q, want 'x'", clip("xy", 1))
	}
}
