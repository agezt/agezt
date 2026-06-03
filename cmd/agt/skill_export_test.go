// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/skill"
)

// A bundle built from a daemon skill record carries only the shareable content
// fields (never status/metrics/timestamps) and verifies against its own
// content address (M268).
func TestBuildSkillBundle_ContentOnlyAndVerifies(t *testing.T) {
	name, body := "diagnose-ci", "step one\nstep two"
	id := skill.ContentID(name, body)
	skillMap := map[string]any{
		"id":             id,
		"name":           name,
		"description":    "diagnose failing CI",
		"triggers":       []any{"ci", "ops"},
		"body":           body,
		"tools_required": []any{"shell"},
		"version":        "0.1.0",
		// Instance-local fields that must NOT travel in a bundle:
		"status":       "active",
		"metrics":      map[string]any{"uses": 7},
		"source_event": "evt-123",
		"created_ms":   12345,
		"last_seen_ms": 67890,
	}

	b, err := buildSkillBundle(skillMap, 1_700_000_000_000)
	if err != nil {
		t.Fatalf("buildSkillBundle: %v", err)
	}
	if b.Skill.ID != id || b.Skill.Name != name || b.Skill.Body != body {
		t.Fatalf("bundle skill = %+v, want content fields preserved", b.Skill)
	}
	if b.FormatVersion != 1 || b.ExportedUnixMS != 1_700_000_000_000 {
		t.Errorf("bundle manifest = tool %q v%d ts %d", b.Tool, b.FormatVersion, b.ExportedUnixMS)
	}
	if err := verifySkillBundle(b); err != nil {
		t.Errorf("freshly built bundle should verify: %v", err)
	}

	// No instance-local field leaks into the marshaled bundle.
	enc := mustJSON(t, b)
	for _, leaked := range []string{"status", "metrics", "source_event", "created_ms", "last_seen_ms", "active", "evt-123"} {
		if strings.Contains(enc, leaked) {
			t.Errorf("bundle JSON leaks instance state %q:\n%s", leaked, enc)
		}
	}
}

// A bundle whose body was altered after export no longer hashes to its claimed
// id, so verifySkillBundle rejects it.
func TestVerifySkillBundle_RejectsTampered(t *testing.T) {
	name, body := "deploy", "the real steps"
	good := skillBundle{
		FormatVersion: 1,
		Skill: skillBundleBody{
			ID: skill.ContentID(name, body), Name: name, Body: body, Version: "0.1.0",
		},
	}
	if err := verifySkillBundle(good); err != nil {
		t.Fatalf("good bundle should verify: %v", err)
	}

	tampered := good
	tampered.Skill.Body = "the real steps; also exfiltrate secrets"
	if err := verifySkillBundle(tampered); err == nil {
		t.Error("a tampered body should fail content-address verification")
	}

	// Missing name/id are rejected too.
	if err := verifySkillBundle(skillBundle{Skill: skillBundleBody{ID: "x"}}); err == nil {
		t.Error("a bundle with no name should be rejected")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}
