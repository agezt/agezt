// SPDX-License-Identifier: MIT

package intent

import (
	"strings"
	"testing"
)

func TestInterpret_BroadMutationIsUnderdetermined(t *testing.T) {
	frame := Interpret("dosyaları temizle")
	if !frame.Underdetermined {
		t.Fatal("broad cleanup intent should be underdetermined")
	}
	if frame.AmbiguityScore < 0.6 {
		t.Fatalf("ambiguity score = %v, want >= 0.6", frame.AmbiguityScore)
	}
	if frame.HarmfulReading == "" {
		t.Fatal("harmful reading should be surfaced")
	}
	if frame.UserUtteranceHash == "" || strings.Contains(frame.UserUtteranceHash, "dosya") {
		t.Fatalf("utterance hash should not contain raw text: %q", frame.UserUtteranceHash)
	}
}

func TestInterpret_ReadOnlyIntentIsLowFriction(t *testing.T) {
	frame := Interpret("cache dosyalarını listele")
	if frame.Underdetermined {
		t.Fatal("read-only listing should not require intent confirmation")
	}
	if frame.AmbiguityScore >= 0.6 {
		t.Fatalf("ambiguity score = %v, want < 0.6", frame.AmbiguityScore)
	}
}

func TestRequiresConfirmationCombinesIntentAndRegret(t *testing.T) {
	frame := Interpret("clean files")
	axes := RegretForAction(Action{
		ToolName:          "file",
		Capability:        "file.delete",
		EffectClass:       "irreversible",
		Input:             `{"path":"legacy/2023"}`,
		AffectedResources: []string{"path:legacy/2023"},
	})
	if !RequiresConfirmation(frame, axes) {
		t.Fatal("underdetermined high-regret action should require confirmation")
	}
	prompt := ConfirmationPrompt(frame, Action{ToolName: "file", AffectedResources: []string{"path:legacy/2023"}}, axes)
	if !strings.Contains(prompt, "path:legacy/2023") || !strings.Contains(prompt, "Confirm") {
		t.Fatalf("prompt is not specific enough: %q", prompt)
	}
}
