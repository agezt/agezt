// SPDX-License-Identifier: MIT

package provopts

import (
	"encoding/json"
	"testing"
)

func TestMerge_EmptyExtraUnchanged(t *testing.T) {
	body := []byte(`{"model":"x","max_tokens":10}`)
	for _, extra := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("   ")} {
		got, err := Merge(body, extra)
		if err != nil {
			t.Fatalf("Merge err: %v", err)
		}
		if string(got) != string(body) {
			t.Fatalf("empty extra changed body: %s", got)
		}
	}
}

func TestMerge_Overlay(t *testing.T) {
	body := []byte(`{"model":"x","temperature":0.1}`)
	got, err := Merge(body, json.RawMessage(`{"temperature":0.9,"logprobs":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["temperature"] != 0.9 {
		t.Fatalf("overlay did not win: %v", m["temperature"])
	}
	if m["logprobs"] != true {
		t.Fatalf("overlay key missing: %v", m)
	}
	if m["model"] != "x" {
		t.Fatalf("base key lost: %v", m)
	}
}

func TestThinkingBudget(t *testing.T) {
	if b, ok := ThinkingBudget("", 0); ok || b != 0 {
		t.Fatalf("empty effort should be (0,false), got (%d,%v)", b, ok)
	}
	if b, ok := ThinkingBudget("high", 0); !ok || b != 16384 {
		t.Fatalf("high: got (%d,%v)", b, ok)
	}
	// Cap below max_tokens.
	if b, ok := ThinkingBudget("high", 5000); !ok || b != 4999 {
		t.Fatalf("capped: got (%d,%v)", b, ok)
	}
	// max_tokens too small to carry a valid budget → disabled.
	if b, ok := ThinkingBudget("low", 1024); ok || b != 0 {
		t.Fatalf("tiny max: got (%d,%v)", b, ok)
	}
}

func TestNormalizeEffort(t *testing.T) {
	for in, want := range map[string]string{
		"HIGH": "high", " low ": "low", "medium": "medium",
		"minimal": "minimal", "bogus": "", "": "",
	} {
		if got := NormalizeEffort(in); got != want {
			t.Fatalf("NormalizeEffort(%q)=%q want %q", in, got, want)
		}
	}
}
