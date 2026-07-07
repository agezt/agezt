// SPDX-License-Identifier: MIT

package browser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBrowserCoverageActionVerbToolsAndDefinition(t *testing.T) {
	// NewActionVerbTools: nil base returns nil.
	if got := NewActionVerbTools(nil); got != nil {
		t.Fatalf("NewActionVerbTools(nil) = %v, want nil", got)
	}
	// Non-nil base: returns one tool per known verb.
	base := &ActionTool{NodePath: "node", DriverPath: "x.mjs"}
	tools := NewActionVerbTools(base)
	if len(tools) != 10 {
		t.Fatalf("got %d tools, want 10", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(*ActionVerbTool).Name] = true
	}
	for _, want := range []string{
		ActionVerbOpen, ActionVerbSnapshot, ActionVerbClick, ActionVerbType,
		ActionVerbWait, ActionVerbScreenshot, ActionVerbDownloads,
		ActionVerbCookies, ActionVerbTabs, ActionVerbClose,
	} {
		if !names[want] {
			t.Fatalf("missing verb %q", want)
		}
	}

	// Per-verb Definition covers the schema and effect branches.
	for _, tool := range tools {
		vt := tool.(*ActionVerbTool)
		def := vt.Definition()
		if def.Name != vt.Name {
			t.Fatalf("Definition Name = %q, want %q", def.Name, vt.Name)
		}
		if len(def.InputSchema) == 0 {
			t.Fatalf("Definition for %s missing InputSchema", vt.Name)
		}
		if def.Effect.Confidence <= 0 {
			t.Fatalf("Definition for %s has zero confidence", vt.Name)
		}
	}

	// Invoke with no base: should soft-error.
	tool := &ActionVerbTool{Name: ActionVerbOpen, Base: nil}
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{}`))
	if !res.IsError || !strings.Contains(res.Output, "not configured") {
		t.Fatalf("nil base invoke = %+v", res)
	}

	// Parse error: hard.
	tool = &ActionVerbTool{Name: ActionVerbOpen, Base: base}
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}
}

func TestBrowserCoverageActionVerbConvertAndResolve(t *testing.T) {
	// actionVerbToActionInput: every branch.
	cases := []struct {
		name   string
		verb   string
		in     actionVerbInput
		check  func(t *testing.T, out actionInput)
		isErr  bool
		errMsg string
	}{
		{name: "open", verb: ActionVerbOpen, in: actionVerbInput{URL: "https://x"}, check: func(t *testing.T, out actionInput) {
			if out.Snapshot == nil || *out.Snapshot != true {
				t.Fatal("open should default snapshot=true")
			}
		}},
		{name: "snapshot", verb: ActionVerbSnapshot, in: actionVerbInput{URL: "https://x"}, check: func(t *testing.T, out actionInput) {
			if out.Screenshot == nil || *out.Screenshot != false {
				t.Fatal("snapshot should default screenshot=false")
			}
		}},
		{name: "click-no-selector", verb: ActionVerbClick, in: actionVerbInput{}, isErr: true, errMsg: "selector or ref required"},
		{name: "click", verb: ActionVerbClick, in: actionVerbInput{Selector: "#btn"}, check: func(t *testing.T, out actionInput) {
			if len(out.Actions) != 1 || out.Actions[0].Type != "click" || out.Actions[0].Selector != "#btn" {
				t.Fatalf("click = %+v", out.Actions)
			}
		}},
		{name: "type-no-selector", verb: ActionVerbType, in: actionVerbInput{Value: "x"}, isErr: true, errMsg: "selector or ref required"},
		{name: "type-no-value", verb: ActionVerbType, in: actionVerbInput{Selector: "#in"}, isErr: true, errMsg: "value required"},
		{name: "type", verb: ActionVerbType, in: actionVerbInput{Selector: "#in", Value: "hello", DelayMS: 5}, check: func(t *testing.T, out actionInput) {
			if len(out.Actions) != 1 || out.Actions[0].Value != "hello" || out.Actions[0].DelayMS != 5 {
				t.Fatalf("type = %+v", out.Actions)
			}
		}},
		{name: "type-submit", verb: ActionVerbType, in: actionVerbInput{Selector: "#in", Value: "hi", Submit: true}, check: func(t *testing.T, out actionInput) {
			if len(out.Actions) != 2 || out.Actions[1].Key != "Enter" {
				t.Fatalf("type submit = %+v", out.Actions)
			}
		}},
		{name: "type-submit-custom-key", verb: ActionVerbType, in: actionVerbInput{Selector: "#in", Value: "hi", Submit: true, Key: "Tab"}, check: func(t *testing.T, out actionInput) {
			if len(out.Actions) != 2 || out.Actions[1].Key != "Tab" {
				t.Fatalf("type submit custom = %+v", out.Actions)
			}
		}},
		{name: "wait", verb: ActionVerbWait, in: actionVerbInput{WaitSelector: "#x", WaitMS: 200}, check: func(t *testing.T, out actionInput) {
			if len(out.Actions) != 1 || out.Actions[0].Type != "wait" || out.Actions[0].MS != 200 {
				t.Fatalf("wait = %+v", out.Actions)
			}
		}},
		{name: "screenshot", verb: ActionVerbScreenshot, in: actionVerbInput{URL: "https://x"}, check: func(t *testing.T, out actionInput) {
			if out.Screenshot == nil || *out.Screenshot != true {
				t.Fatal("screenshot should default true")
			}
		}},
		{name: "downloads", verb: ActionVerbDownloads, in: actionVerbInput{URL: "https://x"}, check: func(t *testing.T, out actionInput) {
			if out.Downloads == nil || *out.Downloads != true {
				t.Fatal("downloads should default true")
			}
		}},
		{name: "downloads-with-selector", verb: ActionVerbDownloads, in: actionVerbInput{URL: "https://x", Selector: "#dl"}, check: func(t *testing.T, out actionInput) {
			if len(out.Actions) != 1 || out.Actions[0].Type != "click" {
				t.Fatalf("downloads with selector = %+v", out.Actions)
			}
		}},
		{name: "cookies", verb: ActionVerbCookies, in: actionVerbInput{URL: "https://x"}, check: func(t *testing.T, out actionInput) {
			if !out.Cookies {
				t.Fatal("cookies should be true")
			}
		}},
		{name: "tabs-rejected", verb: ActionVerbTabs, in: actionVerbInput{}, isErr: true, errMsg: "handled without browser.action conversion"},
		{name: "close-rejected", verb: ActionVerbClose, in: actionVerbInput{}, isErr: true, errMsg: "handled without browser.action conversion"},
		{name: "unknown", verb: "browser.frobnicate", in: actionVerbInput{}, isErr: true, errMsg: "unknown browser verb"},
		{name: "with-wait-suffix", verb: ActionVerbOpen, in: actionVerbInput{URL: "https://x", WaitSelector: "#x", WaitMS: 100}, check: func(t *testing.T, out actionInput) {
			// Last action should be a wait.
			last := out.Actions[len(out.Actions)-1]
			if last.Type != "wait" || last.Selector != "#x" || last.MS != 100 {
				t.Fatalf("with-wait suffix = %+v", out.Actions)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := actionVerbToActionInput(tc.verb, tc.in)
			if tc.isErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (out=%+v)", tc.errMsg, got)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestBrowserCoverageActionVerbDefaultBoolAndAppendOptionalWait(t *testing.T) {
	// defaultBool: nil → fallback; non-nil → value.
	t1 := true
	if got := defaultBool(nil, true); got == nil || *got != true {
		t.Fatalf("defaultBool(nil,true) = %v", got)
	}
	if got := defaultBool(&t1, false); got == nil || *got != true {
		t.Fatalf("defaultBool(&true, false) = %v", got)
	}

	// appendOptionalWait: with both wait_selector and wait_ms, appends a wait.
	in := actionVerbInput{WaitSelector: "#x", WaitMS: 50}
	steps := appendOptionalWait([]browserStep{}, in)
	if len(steps) != 1 || steps[0].Type != "wait" || steps[0].MS != 50 {
		t.Fatalf("with both = %+v", steps)
	}
	// With only wait_selector, appends.
	in2 := actionVerbInput{WaitSelector: "#y"}
	steps2 := appendOptionalWait(nil, in2)
	if len(steps2) != 1 || steps2[0].Selector != "#y" {
		t.Fatalf("with only selector = %+v", steps2)
	}
	// With only wait_ms, appends.
	in3 := actionVerbInput{WaitMS: 10}
	steps3 := appendOptionalWait(nil, in3)
	if len(steps3) != 1 || steps3[0].MS != 10 {
		t.Fatalf("with only ms = %+v", steps3)
	}
	// With neither, returns steps as-is.
	in4 := actionVerbInput{}
	steps4 := appendOptionalWait([]browserStep{{Type: "noop"}}, in4)
	if len(steps4) != 1 || steps4[0].Type != "noop" {
		t.Fatalf("with neither = %+v", steps4)
	}
}

func TestBrowserCoverageActionVerbRefResolution(t *testing.T) {
	// resolveRef is a package-level method on *ActionVerbTool. We'll use a real
	// ActionTool with stubbed dependencies via reflection-light fields.
	base := &ActionTool{
		NodePath:   "node",
		DriverPath: "x.mjs",
		run: func(_ context.Context, _ actionRunSpec) (actionRunOutput, error) {
			return actionRunOutput{}, nil
		},
	}
	// Test ref resolution: ref empty → no-op.
	tool := &ActionVerbTool{Name: ActionVerbClick, Base: base}
	in := actionVerbInput{Selector: "#btn"}
	if err := tool.resolveRef(&in); err != nil {
		t.Fatalf("empty ref resolve: %v", err)
	}
	if in.Selector != "#btn" {
		t.Fatalf("selector changed: %q", in.Selector)
	}
	// Unknown verb with ref → error.
	tool2 := &ActionVerbTool{Name: ActionVerbOpen, Base: base}
	in2 := actionVerbInput{Ref: "e1"}
	if err := tool2.resolveRef(&in2); err == nil {
		t.Fatal("ref on non-supported verb should error")
	}
}
