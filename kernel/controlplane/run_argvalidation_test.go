// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRun_RejectsWrongTypedArgs — a per-run override arg sent with the wrong JSON
// type is reported as a usage error rather than silently mis-handled (M161). The
// two dangerous cases this guards: a mistyped `dry_run` that would otherwise fall
// through to false and EXECUTE a run the operator meant to preview (spending
// tokens), and a mistyped `tools` that would otherwise scope the run to NO tools.
func TestRun_RejectsWrongTypedArgs(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("hi")))

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"dry_run as string", map[string]any{"intent": "x", "dry_run": "true"}, "dry_run must be a boolean"},
		{"tools as string", map[string]any{"intent": "x", "tools": "shell"}, "tools must be an array"},
		{"tools element non-string", map[string]any{"intent": "x", "tools": []any{"shell", 3.0}}, "tools[1] must be a string"},
		{"model as number", map[string]any{"intent": "x", "model": 5.0}, "model must be a string"},
		{"timeout as number", map[string]any{"intent": "x", "timeout": 30.0}, "timeout must be a string"},
		{"system as number", map[string]any{"intent": "x", "system": 1.0}, "system must be a string"},
		{"images as string", map[string]any{"intent": "x", "images": "photo.png"}, "images must be an array"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Stream(context.Background(), controlplane.CmdRun, tc.args, func(*event.Event) {})
			if err == nil {
				t.Fatalf("expected usage error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q; want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// TestRun_WellTypedArgsStillRun — the correctly-typed forms the CLI actually
// sends are unaffected: a dry_run:true returns a plan (no execution), and an
// ordinary run completes.
func TestRun_WellTypedArgsStillRun(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// dry_run:true → a plan, run not executed.
	plan, err := c.Call(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x", "dry_run": true, "tools": []any{}})
	if err != nil {
		t.Fatalf("dry-run errored: %v", err)
	}
	if plan["dry_run"] != true {
		t.Errorf("plan dry_run = %v, want true", plan["dry_run"])
	}
	if plan["tools_mode"] != "none (--no-tools)" {
		t.Errorf("tools_mode = %v, want none (--no-tools)", plan["tools_mode"])
	}

	// Ordinary run still completes (mock is fresh — the dry-run spent nothing).
	res, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "hello"}, func(*event.Event) {})
	if err != nil {
		t.Fatalf("plain run errored: %v", err)
	}
	if res["answer"] != "ok" {
		t.Errorf("answer = %v, want ok", res["answer"])
	}
}
