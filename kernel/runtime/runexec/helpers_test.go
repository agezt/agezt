// SPDX-License-Identifier: MIT

package runexec

import (
	"errors"
	"strings"
	"testing"
)

func TestErrHalted_IsSentinel(t *testing.T) {
	// ErrHalted must compare with itself and stay distinct from
	// the bare "halted" string callers see in event payloads.
	if !errors.Is(ErrHalted, ErrHalted) {
		t.Errorf("errors.Is(ErrHalted, ErrHalted) = false, want true")
	}
	if errors.Is(errors.New("halted"), ErrHalted) {
		t.Errorf("a fresh error must not satisfy errors.Is(ErrHalted, ...)")
	}
}

func TestBuildTranscript_Empty(t *testing.T) {
	got := buildTranscript(nil, "hello")
	if !strings.Contains(got, "Final answer:\nhello") {
		t.Errorf("transcript missing 'Final answer' section: %q", got)
	}
	if strings.Contains(got, "Tools used:") {
		t.Errorf("empty tool list must not emit 'Tools used:' line: %q", got)
	}
}

func TestBuildTranscript_WithTools(t *testing.T) {
	got := buildTranscript([]string{"shell", "read"}, "answer")
	if !strings.Contains(got, "Tools used: shell, read") {
		t.Errorf("transcript missing tools line: %q", got)
	}
	if !strings.Contains(got, "Final answer:\nanswer") {
		t.Errorf("transcript missing final answer: %q", got)
	}
}

func TestShadowEvalLimit_BoundedByTwo(t *testing.T) {
	// The cap is a single named constant; if it ever changes,
	// downstream LLM cost estimates must be re-derived. A test
	// pin is cheaper than a code-search audit.
	if shadowEvalLimit != 2 {
		t.Errorf("shadowEvalLimit = %d, want 2 (SPEC-05 §5.2)", shadowEvalLimit)
	}
}
