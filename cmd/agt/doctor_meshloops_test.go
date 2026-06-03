// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func TestMeshLoopCheck_NoneRefusedIsQuiet(t *testing.T) {
	// nil map, map without the kind, and an explicit zero all stay silent.
	for name, byKind := range map[string]map[string]any{
		"nil map":       nil,
		"kind absent":   {"run.started": float64(12)},
		"explicit zero": {string(event.KindMeshLoopRefused): float64(0)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, show := meshLoopCheck(byKind); show {
				t.Errorf("expected no check line for %q", name)
			}
		})
	}
}

func TestMeshLoopCheck_RefusalsWarn(t *testing.T) {
	// JSON numbers arrive as float64 across the control-plane wire.
	byKind := map[string]any{string(event.KindMeshLoopRefused): float64(3)}
	c, show := meshLoopCheck(byKind)
	if !show {
		t.Fatal("a non-zero refusal count should surface a check")
	}
	if c.Status != statusWarn {
		t.Fatalf("refusals should WARN, got %s", c.Status.label())
	}
	if !strings.Contains(c.Detail, "3 mesh delegation loop(s) refused") {
		t.Errorf("detail = %q, want the count", c.Detail)
	}
	if c.Hint == "" {
		t.Error("a WARN should carry an investigation hint")
	}
}

func TestMeshLoopCheck_IntegerCountForms(t *testing.T) {
	// intOfStatus also accepts int/int64 (e.g. if called with non-wire data).
	for _, v := range []any{int(2), int64(2), float64(2)} {
		if _, show := meshLoopCheck(map[string]any{string(event.KindMeshLoopRefused): v}); !show {
			t.Errorf("count %T(%v) should surface a warning", v, v)
		}
	}
}
