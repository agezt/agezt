// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

func TestCmdDoctor_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdDoctor([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "doctor [--json]") {
		t.Errorf("help missing usage; got %q", out.String())
	}
}

func TestCmdDoctor_RejectsBadArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdDoctor([]string{"--nope"}, &out, &errOut); code != 2 {
		t.Errorf("unknown arg should be exit 2, got %d", code)
	}
}

// checkBaseDir is the one diagnostic that runs without a daemon — exercise its
// branches directly so the check logic is covered regardless of environment.
func TestCheckBaseDir(t *testing.T) {
	t.Run("writable dir → OK", func(t *testing.T) {
		c := checkBaseDir(t.TempDir(), nil)
		if c.Status != statusOK {
			t.Errorf("writable temp dir should be OK, got %s: %s", c.Status.label(), c.Detail)
		}
	})

	t.Run("resolve error → FAIL", func(t *testing.T) {
		c := checkBaseDir("", errFake("boom"))
		if c.Status != statusFail {
			t.Errorf("resolve error should be FAIL, got %s", c.Status.label())
		}
	})

	t.Run("missing dir → WARN", func(t *testing.T) {
		c := checkBaseDir(t.TempDir()+"/does-not-exist", nil)
		if c.Status != statusWarn {
			t.Errorf("absent dir should be WARN, got %s", c.Status.label())
		}
	})
}

// TestDoctorSummaryExit verifies the exit-code contract: warnings don't fail,
// a FAIL does.
func TestDoctorSummaryExit(t *testing.T) {
	cases := []struct {
		name   string
		checks []doctorCheck
		want   int
	}{
		{"all ok", []doctorCheck{ok("a", "x"), ok("b", "y")}, 0},
		{"a warning", []doctorCheck{ok("a", "x"), warn("b", "y", "h")}, 0},
		{"a failure", []doctorCheck{ok("a", "x"), fail("b", "y", "h")}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := renderDoctorText(tc.checks, &out); got != tc.want {
				t.Errorf("exit = %d want %d", got, tc.want)
			}
		})
	}
}

func TestDoctorJSONShape(t *testing.T) {
	var out bytes.Buffer
	code := renderDoctorJSON([]doctorCheck{ok("a", "x"), fail("b", "y", "h")}, &out)
	if code != 1 {
		t.Errorf("a FAIL → exit 1, got %d", code)
	}
	s := out.String()
	for _, want := range []string{`"healthy"`, `"checks"`, `"status": "FAIL"`, `"hint": "h"`} {
		if !strings.Contains(s, want) {
			t.Errorf("json missing %q in:\n%s", want, s)
		}
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func TestCheckModelReadiness(t *testing.T) {
	cat := catalog.NewEmpty()
	cat.Providers["acme"] = &catalog.Provider{
		ID: "acme", NPM: "@ai-sdk/openai-compatible",
		Models: map[string]*catalog.Model{
			"mini":  {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 32768}},
			"large": {ID: "large", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	}

	// Tool-capable model → OK.
	if c := checkModelReadiness(map[string]any{"model": "large"}, cat); c.Status != statusOK {
		t.Errorf("large should be OK; got %s %q", c.State, c.Detail)
	}
	// Tool-less known model → WARN with the advisory + hint.
	c := checkModelReadiness(map[string]any{"model": "mini"}, cat)
	if c.Status != statusWarn {
		t.Errorf("mini should WARN; got %s", c.State)
	}
	if !strings.Contains(c.Detail, "tool-use") || c.Hint == "" {
		t.Errorf("mini warn should mention tool-use + carry a hint; got %q / %q", c.Detail, c.Hint)
	}
	// Unknown-to-catalog model → OK (no false alarm).
	if c := checkModelReadiness(map[string]any{"model": "ghost"}, cat); c.Status != statusOK {
		t.Errorf("unknown model should be OK; got %s", c.State)
	}
	// Mock / empty model → OK.
	for _, m := range []string{"", "mock"} {
		if c := checkModelReadiness(map[string]any{"model": m}, cat); c.Status != statusOK {
			t.Errorf("model %q should be OK; got %s", m, c.State)
		}
	}
	// Nil catalog → OK (capabilities unknown).
	if c := checkModelReadiness(map[string]any{"model": "mini"}, nil); c.Status != statusOK {
		t.Errorf("nil catalog should be OK; got %s", c.State)
	}
}
