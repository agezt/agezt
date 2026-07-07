// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
)

func TestCoveragePureFormattingHelpers(t *testing.T) {
	if got := pct(25, 100); got != "25" {
		t.Fatalf("pct = %q, want 25", got)
	}
	if got := pct(10, 0); got != "—" {
		t.Fatalf("pct with zero cap = %q", got)
	}
	if got := reasonOrPlaceholder(""); got != "—" {
		t.Fatalf("empty reason = %q", got)
	}
	if got := reasonOrPlaceholder("operator requested"); got != "operator requested" {
		t.Fatalf("reason = %q", got)
	}
	if got := formatTime(""); got != "never" {
		t.Fatalf("empty time = %q", got)
	}
	if got := formatTime("0001-01-01T00:00:00Z"); got != "never" {
		t.Fatalf("zero time = %q", got)
	}
	if got := formatTime("2026-07-07T00:00:00Z"); got != "2026-07-07T00:00:00Z" {
		t.Fatalf("real time = %q", got)
	}
	if got := jsonOrString("plain"); got != "plain" {
		t.Fatalf("jsonOrString string = %q", got)
	}
	if got := jsonOrString(map[string]any{"b": 2}); got != `{"b":2}` {
		t.Fatalf("jsonOrString map = %q", got)
	}
}

func TestCoverageProviderCheckHelpers(t *testing.T) {
	summary := summaryFromProbes(
		jsonProbe{OK: true},
		jsonProbe{OK: false},
		jsonProbe{OK: true},
	)
	if summary.Total != 3 || summary.OK != 2 || summary.Failed != 1 {
		t.Fatalf("summaryFromProbes = %+v", summary)
	}
	if got := computeCostMicrocents(nil, agent.Usage{InputTokens: 10, OutputTokens: 20}); got != 0 {
		t.Fatalf("nil model cost = %d", got)
	}
	model := &catalog.Model{Cost: &catalog.Cost{Input: 2, Output: 3}}
	gotCost := computeCostMicrocents(model, agent.Usage{InputTokens: 1_000_000, OutputTokens: 2_000_000})
	if gotCost != 8_000_000_000 {
		t.Fatalf("computeCostMicrocents = %d, want 8000000000", gotCost)
	}
	cases := map[int64]string{
		0:             "0.00",
		500_000:       "0.0005",
		1_000_000:     "0.001",
		1_000_000_000: "1.00",
		-500_000:      "-0.0005",
	}
	for in, want := range cases {
		if got := formatMicrocentsUSD(in); got != want {
			t.Fatalf("formatMicrocentsUSD(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestCoverageOKRHelpers(t *testing.T) {
	idx := 0
	var stderr bytes.Buffer
	value, ok := okrFlagValue([]string{"--owner", "ops"}, &idx, "--owner", &stderr)
	if !ok || value != "ops" || idx != 1 {
		t.Fatalf("okrFlagValue = %q %v idx=%d stderr=%q", value, ok, idx, stderr.String())
	}
	idx = 0
	stderr.Reset()
	if value, ok := okrFlagValue([]string{"--owner"}, &idx, "--owner", &stderr); ok || value != "" || !strings.Contains(stderr.String(), "needs a value") {
		t.Fatalf("missing okrFlagValue = %q %v stderr=%q", value, ok, stderr.String())
	}

	id, asJSON, ok := okrIDArg([]string{"--json", "okr-1"}, "show", &stderr)
	if !ok || !asJSON || id != "okr-1" {
		t.Fatalf("okrIDArg json = id=%q json=%v ok=%v", id, asJSON, ok)
	}
	stderr.Reset()
	if _, _, ok := okrIDArg([]string{"--bad"}, "show", &stderr); ok || !strings.Contains(stderr.String(), "unexpected flag") {
		t.Fatalf("okrIDArg bad flag ok=%v stderr=%q", ok, stderr.String())
	}
	stderr.Reset()
	if _, _, ok := okrIDArg(nil, "show", &stderr); ok || !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("okrIDArg missing ok=%v stderr=%q", ok, stderr.String())
	}

	obj := map[string]any{"id": "objective-123456789", "status": "active", "percent": float64(42), "title": "Ship coverage"}
	var out bytes.Buffer
	renderOKRLine(&out, obj)
	if line := out.String(); !strings.Contains(line, "active") || !strings.Contains(line, "42%") || !strings.Contains(line, "Ship coverage") {
		t.Fatalf("renderOKRLine = %q", line)
	}
	out.Reset()
	renderOKRObjective(&out, map[string]any{
		"id":          "okr-1",
		"title":       "Coverage",
		"status":      "active",
		"percent":     float64(50),
		"description": "raise coverage",
		"owner":       "qa",
		"progress": map[string]any{"key_results": []any{
			map[string]any{"title": "cmd", "done": float64(1), "total": float64(2), "target": float64(2), "percent": float64(50), "achieved": true},
		}},
	})
	text := out.String()
	for _, want := range []string{"id:", "Coverage", "owner:", "key results:", "✓"} {
		if !strings.Contains(text, want) {
			t.Fatalf("renderOKRObjective missing %q in %q", want, text)
		}
	}
}
