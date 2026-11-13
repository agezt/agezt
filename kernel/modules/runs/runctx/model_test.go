// SPDX-License-Identifier: MIT

package runctx

import (
	"context"
	"testing"
)

// A per-run runctx.WithModel pick beats the host's default and is the only
// one reported EXPLICIT — that flag is what stops the governor's per-task
// chain from rerouting an operator's deliberate choice (M931).
//
// The rule moved here from the daemon in W3.76al: the per-run pick is the run
// context's own vocabulary, and the run engine — which lives inside the runs
// module — used to ask the HOST to apply it.
func TestResolveModel_PerRunPickWinsAndIsExplicit(t *testing.T) {
	model, explicit := ResolveModel(context.Background(), func() string { return "effective-model" })
	if model != "effective-model" || explicit {
		t.Errorf("no per-run pick = (%q, %v), want (effective-model, false)", model, explicit)
	}

	model, explicit = ResolveModel(WithModel(context.Background(), "asked-for"), func() string { return "effective-model" })
	if model != "asked-for" || !explicit {
		t.Errorf("per-run pick = (%q, %v), want (asked-for, true)", model, explicit)
	}
}

// The host's default is asked for LAZILY: a run that carries its own model
// never needs the host's config — and the host's config is a copy taken under
// a lock.
func TestResolveModel_AsksTheHostOnlyWhenItMust(t *testing.T) {
	asked := 0
	defaultModel := func() string {
		asked++
		return "effective-model"
	}

	if _, explicit := ResolveModel(WithModel(context.Background(), "asked-for"), defaultModel); !explicit {
		t.Error("a per-run pick must be explicit")
	}
	if asked != 0 {
		t.Errorf("the host's default was asked %d times, want 0 — a run with its own model never needs the host's config", asked)
	}

	if _, explicit := ResolveModel(context.Background(), defaultModel); explicit {
		t.Error("the host's default must not be reported as explicit")
	}
	if asked != 1 {
		t.Errorf("the host's default was asked %d times, want 1", asked)
	}
}

// An empty per-run pick is not a pick: the host's default answers.
func TestResolveModel_AnEmptyPickIsNoPick(t *testing.T) {
	model, explicit := ResolveModel(WithModel(context.Background(), ""), func() string { return "effective-model" })
	if model != "effective-model" || explicit {
		t.Errorf("empty pick = (%q, %v), want (effective-model, false)", model, explicit)
	}
}
